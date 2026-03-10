package intelligence

import (
	"database/sql"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// CodeIndex provides code intelligence capabilities
type CodeIndex struct {
	DB     *sql.DB
	DBPath string
}

// Package represents a Go package in the index
type Package struct {
	Path     string
	Name     string
	Files    []string
	Imports  []string
	Exported []string
}

// File represents a Go source file
type File struct {
	Path      string
	Package   string
	Imports   []string
	Types     []string
	Functions []string
	Methods   []Method
}

// Method represents a method on a type
type Method struct {
	Name     string
	Receiver string
	File     string
}

// NewCodeIndex creates or opens a code intelligence database
func NewCodeIndex(dbPath string) (*CodeIndex, error) {
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open code index: %w", err)
	}

	ci := &CodeIndex{
		DB:     db,
		DBPath: dbPath,
	}

	if err := ci.InitSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return ci, nil
}

// InitSchema creates the code intelligence tables
func (ci *CodeIndex) InitSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS packages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT UNIQUE NOT NULL,
		package_id INTEGER,
		FOREIGN KEY (package_id) REFERENCES packages(id)
	);

	CREATE TABLE IF NOT EXISTS imports (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id INTEGER,
		path TEXT NOT NULL,
		FOREIGN KEY (file_id) REFERENCES files(id)
	);

	CREATE TABLE IF NOT EXISTS types (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id INTEGER,
		name TEXT NOT NULL,
		kind TEXT NOT NULL, -- struct, interface, etc.
		FOREIGN KEY (file_id) REFERENCES files(id)
	);

	CREATE TABLE IF NOT EXISTS functions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id INTEGER,
		name TEXT NOT NULL,
		receiver TEXT, -- NULL for regular functions, type name for methods
		FOREIGN KEY (file_id) REFERENCES files(id)
	);

	CREATE TABLE IF NOT EXISTS dependencies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_file TEXT NOT NULL,
		to_file TEXT NOT NULL,
		direction TEXT NOT NULL -- import, call, type_usage
	);

	CREATE INDEX IF NOT EXISTS idx_packages_path ON packages(path);
	CREATE INDEX IF NOT EXISTS idx_files_package ON files(package_id);
	CREATE INDEX IF NOT EXISTS idx_functions_file ON functions(file_id);
	CREATE INDEX IF NOT EXISTS idx_dependencies_from ON dependencies(from_file);
	CREATE INDEX IF NOT EXISTS idx_dependencies_to ON dependencies(to_file);
	`

	_, err := ci.DB.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}
	return nil
}

// IndexProject scans the project and builds the code index
func (ci *CodeIndex) IndexProject(projectPath string) error {
	// Clear existing data
	if _, err := ci.DB.Exec("DELETE FROM dependencies"); err != nil {
		return err
	}
	if _, err := ci.DB.Exec("DELETE FROM functions"); err != nil {
		return err
	}
	if _, err := ci.DB.Exec("DELETE FROM types"); err != nil {
		return err
	}
	if _, err := ci.DB.Exec("DELETE FROM imports"); err != nil {
		return err
	}
	if _, err := ci.DB.Exec("DELETE FROM files"); err != nil {
		return err
	}
	if _, err := ci.DB.Exec("DELETE FROM packages"); err != nil {
		return err
	}

	// Walk project directory
	return filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip vendor, .git, .dwight, etc.
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process Go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		return ci.indexFile(path, projectPath)
	})
}

// indexFile parses and indexes a single Go file
func (ci *CodeIndex) indexFile(filePath, projectPath string) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		// Log but don't fail - some files might have parse errors
		return nil
	}

	// Get or create package
	pkgPath := filepath.Dir(filePath)
	relPath, _ := filepath.Rel(projectPath, pkgPath)

	var pkgID int64
	err = ci.DB.QueryRow(
		"INSERT OR IGNORE INTO packages (path, name) VALUES (?, ?) RETURNING id",
		relPath, node.Name.Name,
	).Scan(&pkgID)

	if err != nil {
		// Package already exists, get its ID
		err = ci.DB.QueryRow(
			"SELECT id FROM packages WHERE path = ?",
			relPath,
		).Scan(&pkgID)
		if err != nil {
			return err
		}
	}

	// Insert file
	relFilePath, _ := filepath.Rel(projectPath, filePath)
	result, err := ci.DB.Exec(
		"INSERT INTO files (path, package_id) VALUES (?, ?)",
		relFilePath, pkgID,
	)
	if err != nil {
		return err
	}
	fileID, _ := result.LastInsertId()

	// Index imports
	for _, imp := range node.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		_, err := ci.DB.Exec(
			"INSERT INTO imports (file_id, path) VALUES (?, ?)",
			fileID, path,
		)
		if err != nil {
			return err
		}
	}

	// Index declarations
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					kind := "unknown"
					switch s.Type.(type) {
					case *ast.StructType:
						kind = "struct"
					case *ast.InterfaceType:
						kind = "interface"
					}
					_, err := ci.DB.Exec(
						"INSERT INTO types (file_id, name, kind) VALUES (?, ?, ?)",
						fileID, s.Name.Name, kind,
					)
					if err != nil {
						return err
					}
				}
			}

		case *ast.FuncDecl:
			receiver := ""
			if d.Recv != nil && len(d.Recv.List) > 0 {
				// Extract receiver type name
				switch t := d.Recv.List[0].Type.(type) {
				case *ast.Ident:
					receiver = t.Name
				case *ast.StarExpr:
					if ident, ok := t.X.(*ast.Ident); ok {
						receiver = ident.Name
					}
				}
			}

			_, err := ci.DB.Exec(
				"INSERT INTO functions (file_id, name, receiver) VALUES (?, ?, ?)",
				fileID, d.Name.Name, receiver,
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// SearchDependencies finds all files that depend on or are depended upon by a given file
func (ci *CodeIndex) SearchDependencies(filePath string) ([]string, error) {
	query := `
		SELECT DISTINCT from_file FROM dependencies WHERE to_file = ?
		UNION
		SELECT DISTINCT to_file FROM dependencies WHERE from_file = ?
	`
	rows, err := ci.DB.Query(query, filePath, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	return files, nil
}

// GetFileInfo retrieves detailed information about a file
func (ci *CodeIndex) GetFileInfo(filePath string) (*File, error) {
	// Get basic file info
	var file File
	var pkgID int64
	err := ci.DB.QueryRow(
		"SELECT id, package_id FROM files WHERE path = ?",
		filePath,
	).Scan(&file.Path, &pkgID)
	if err != nil {
		return nil, err
	}
	file.Path = filePath

	// Get package name
	ci.DB.QueryRow(
		"SELECT name FROM packages WHERE id = ?",
		pkgID,
	).Scan(&file.Package)

	// Get imports
	rows, err := ci.DB.Query(
		"SELECT path FROM imports WHERE file_id = (SELECT id FROM files WHERE path = ?)",
		filePath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var imp string
		if err := rows.Scan(&imp); err != nil {
			return nil, err
		}
		file.Imports = append(file.Imports, imp)
	}

	// Get types
	rows, err = ci.DB.Query(
		"SELECT name FROM types WHERE file_id = (SELECT id FROM files WHERE path = ?)",
		filePath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		file.Types = append(file.Types, t)
	}

	// Get functions
	rows, err = ci.DB.Query(
		"SELECT name, receiver FROM functions WHERE file_id = (SELECT id FROM files WHERE path = ?)",
		filePath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name, receiver string
		if err := rows.Scan(&name, &receiver); err != nil {
			return nil, err
		}
		if receiver == "" {
			file.Functions = append(file.Functions, name)
		} else {
			file.Methods = append(file.Methods, Method{
				Name:     name,
				Receiver: receiver,
				File:     filePath,
			})
		}
	}

	return &file, nil
}

// GetAllPackages retrieves all packages from the index
func (ci *CodeIndex) GetAllPackages() ([]Package, error) {
	query := `
		SELECT path, name FROM packages ORDER BY path ASC
	`
	rows, err := ci.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var packages []Package
	for rows.Next() {
		var pkg Package
		if err := rows.Scan(&pkg.Path, &pkg.Name); err != nil {
			return nil, err
		}
		packages = append(packages, pkg)
	}

	for i := range packages {
		expRows, err := ci.DB.Query(
			"SELECT name FROM functions WHERE file_id IN (SELECT id FROM files WHERE package_id = (SELECT id FROM packages WHERE path = ?)) AND receiver = ''",
			packages[i].Path,
		)
		if err == nil {
			defer expRows.Close()
			for expRows.Next() {
				var name string
				if err := expRows.Scan(&name); err == nil {
					packages[i].Exported = append(packages[i].Exported, name)
				}
			}
		}
	}

	return packages, nil
}

// GetPackageFiles retrieves all file paths for a given package
func (ci *CodeIndex) GetPackageFiles(pkgPath string) ([]string, error) {
	query := `
		SELECT f.path FROM files f
		JOIN packages p ON f.package_id = p.id
		WHERE p.path = ?
		ORDER BY f.path ASC
	`
	rows, err := ci.DB.Query(query, pkgPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	return files, nil
}

// Close closes the database connection
func (ci *CodeIndex) Close() error {
	return ci.DB.Close()
}
