package main

import (
	// Import necessary packages
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Constants for the Tree Sitter version and download URL
const sitterVersion = "0.22.5"
const sitterURL = "https://github.com/tree-sitter/tree-sitter/archive/refs/tags/v" + sitterVersion + ".tar.gz"

func main() {
	// Get the current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Error getting current directory: %v", err)
	}

	// Construct the directory path for the downloaded Tree Sitter files
	treeSitterDir := "tree-sitter-" + sitterVersion
	parentPath := filepath.Join(currentDir, "tmpts", treeSitterDir)

	// Download and extract the Tree Sitter source code
	if err := downloadAndExtractSitter(sitterURL, sitterVersion); err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Copy files to tmpts directory, preserving subdirectory structure
	// to avoid header name collisions with system headers (e.g. endian.h).
	// Subdirectories are discovered automatically so the script doesn't need
	// updating when upstream adds new ones.
	tmpDir := filepath.Join(currentDir, "tmpts")
	copyDirContents(filepath.Join(parentPath, "lib", "include"), tmpDir)
	copyDirContents(filepath.Join(parentPath, "lib", "src"), tmpDir)

	// Remove the original extracted directory
	err = os.RemoveAll(parentPath)
	if err != nil {
		log.Fatalf("Error removing extracted treesitter directory: %v", err)
	}

	// Clean up unnecessary files
	cleanup(filepath.Join(currentDir, "tmpts"))

	// Generate doc.go in subdirectories that only contain C headers, so that
	// go mod vendor will include them. See https://github.com/golang/go/issues/26366
	docDirs, err := generateDocFiles(filepath.Join(currentDir, "tmpts"))
	if err != nil {
		log.Fatalf("Error generating doc.go files: %v", err)
	}

	// Copy and report files from tmpts to two levels up in the directory structure
	dstDir := filepath.Join(currentDir, "..", "..")
	err = copyAndReportFiles(filepath.Join(currentDir, "tmpts"), dstDir)
	if err != nil {
		log.Fatalf("Error copying and reporting files: %v", err)
	}

	err = os.RemoveAll(filepath.Join(currentDir, "tmpts"))
	if err != nil {
		log.Fatalf("Error removing tmpts directory: %v", err)
	}

	// Check bindings.go for missing blank imports of doc.go subdirectories.
	checkBlankImports(filepath.Join(dstDir, "bindings.go"), docDirs)

	fmt.Printf("\n\nDone!\n")
}

// Function to copy and report files from source to destination directory
func copyAndReportFiles(srcDir, dstDir string) error {
	// Walk through the source directory
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Calculate relative file path and destination file path
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dstFilePath := filepath.Join(dstDir, relPath)

		// Check if file exists at destination and print appropriate message
		if _, err := os.Stat(dstFilePath); err == nil {
			fmt.Printf("%-39s %s\n", filepath.Base(dstFilePath), "[replaced]")
		} else if os.IsNotExist(err) {
			fmt.Printf("%-39s %s\n", filepath.Base(dstFilePath), "[new file]")
		}

		// Copy the file to destination
		return copyFile(path, dstFilePath)
	})
}

// copyDirContents copies .c and .h files from srcDir into dstDir, preserving
// any subdirectory structure. Subdirectories are discovered automatically.
func copyDirContents(srcDir, dstDir string) {
	entries, err := ioutil.ReadDir(srcDir)
	if err != nil {
		log.Fatal(err)
	}
	for _, entry := range entries {
		src := filepath.Join(srcDir, entry.Name())
		if entry.IsDir() {
			copyDirContents(src, filepath.Join(dstDir, entry.Name()))
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext == ".c" || ext == ".h" {
			copyFile(src, filepath.Join(dstDir, entry.Name()))
		}
	}
}

// Function to copy a single file from source to destination
func copyFile(src, dst string) error {
	// Read the file from source
	input, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// Write the file to destination
	err = ioutil.WriteFile(dst, input, 0644)
	if err != nil {
		return err
	}
	return nil
}

// generateDocFiles creates a doc.go in each subdirectory of root that contains
// only header files and no Go files. Without a .go file, go mod vendor skips
// the directory. Directories with .c files are excluded because Go would try
// to compile them without proper CGo setup.
// Returns the list of subdirectory names that received a doc.go.
func generateDocFiles(root string) ([]string, error) {
	entries, err := ioutil.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		hasH, hasC, hasGo := false, false, false
		for _, f := range files {
			switch filepath.Ext(f.Name()) {
			case ".h":
				hasH = true
			case ".c":
				hasC = true
			case ".go":
				hasGo = true
			}
		}
		if hasH && !hasC && !hasGo {
			content := fmt.Sprintf(
				"// Package %s contains C header files for tree-sitter.\n"+
					"// This file exists solely to ensure go mod vendor copies this directory.\n"+
					"// See: https://github.com/golang/go/issues/26366\n"+
					"package %s\n",
				entry.Name(), entry.Name())
			path := filepath.Join(dir, "doc.go")
			if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
				return nil, err
			}
			fmt.Printf("%-39s %s\n", "doc.go ("+entry.Name()+")", "[generated]")
			dirs = append(dirs, entry.Name())
		}
	}
	return dirs, nil
}

// checkBlankImports reads bindings.go and warns about any doc.go subdirectories
// that are missing a blank import. Without the blank import, go mod vendor will
// not copy the subdirectory even though it contains a doc.go.
func checkBlankImports(bindingsPath string, docDirs []string) {
	content, err := ioutil.ReadFile(bindingsPath)
	if err != nil {
		return
	}
	src := string(content)
	var missing []string
	for _, dir := range docDirs {
		// Look for a blank import like: _ ".../<dir>"
		if !strings.Contains(src, "/"+dir+"\"") {
			missing = append(missing, dir)
		}
	}
	if len(missing) > 0 {
		fmt.Printf("\nWARNING: The following subdirectories have doc.go but no blank import in bindings.go:\n")
		for _, dir := range missing {
			fmt.Printf("  _ \"github.com/api-extraction-examples/go-tree-sitter/%s\"\n", dir)
		}
		fmt.Printf("Add the above import(s) to bindings.go so that go mod vendor copies these directories.\n")
	}
}

// Function to download and extract Tree Sitter from the given URL
func downloadAndExtractSitter(url, version string) error {
	// Send HTTP request to download the file
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Prepare gzip reader
	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gzr.Close()

	// Prepare tar reader and extract files
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Process files within specified directories
		if !strings.HasPrefix(header.Name, "tree-sitter-"+version+"/lib/src") && !strings.HasPrefix(header.Name, "tree-sitter-"+version+"/lib/include") {
			continue
		}

		relPath := strings.TrimPrefix(header.Name, version+"/")
		target := filepath.Join("tmpts", relPath)

		// Create directories and files as needed
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// Function to clean up the specified directory
func cleanup(path string) {
	// Walk through the directory and remove unnecessary files
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".h" && filepath.Ext(path) != ".c" || filepath.Base(path) == "lib.c" {
			return os.Remove(path)
		}
		return nil
	})

	if err != nil {
		// Handle the error
	}
}

// Function to run a command and pipe its output
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
