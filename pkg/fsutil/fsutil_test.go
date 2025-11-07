package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// TestSuite for fsutil package
type FsutilTestSuite struct {
	suite.Suite
	tempDir string
}

// SetupTest creates a temporary directory for testing
func (suite *FsutilTestSuite) SetupTest() {
	tempDir, err := os.MkdirTemp("", "fsutil_test_*")
	assert.NoError(suite.T(), err)
	suite.tempDir = tempDir
}

// TearDownTest cleans up the temporary directory
func (suite *FsutilTestSuite) TearDownTest() {
	if suite.tempDir != "" {
		os.RemoveAll(suite.tempDir)
	}
}

// TestGetFileMode tests platform-aware file mode selection
func (suite *FsutilTestSuite) TestGetFileMode() {
	mode := GetFileMode()

	if runtime.GOOS == "windows" {
		assert.Equal(suite.T(), os.FileMode(DefaultFilePermsWindows), mode)
	} else {
		assert.Equal(suite.T(), os.FileMode(DefaultFilePermsUnix), mode)
	}
}

// TestGetDirMode tests platform-aware directory mode selection
func (suite *FsutilTestSuite) TestGetDirMode() {
	mode := GetDirMode()

	if runtime.GOOS == "windows" {
		assert.Equal(suite.T(), os.FileMode(DefaultDirPermsWindows), mode)
	} else {
		assert.Equal(suite.T(), os.FileMode(DefaultDirPermsUnix), mode)
	}
}

// TestPathsEqual tests case-insensitive path comparison on Windows
func (suite *FsutilTestSuite) TestPathsEqual() {
	// Test identical paths
	assert.True(suite.T(), PathsEqual("/path/to/file", "/path/to/file"))

	if runtime.GOOS == "windows" {
		// Test case-insensitive comparison on Windows
		assert.True(suite.T(), PathsEqual("C:\\Path\\To\\File", "c:\\path\\to\\file"))
		assert.True(suite.T(), PathsEqual("file.txt", "FILE.TXT"))
		assert.False(suite.T(), PathsEqual("file1.txt", "file2.txt"))
	} else {
		// Test case-sensitive comparison on Unix-like systems
		assert.False(suite.T(), PathsEqual("/Path/To/File", "/path/to/file"))
		assert.False(suite.T(), PathsEqual("file.txt", "FILE.TXT"))
	}
}

// TestSafeJoin tests path joining with normalization
func (suite *FsutilTestSuite) TestSafeJoin() {
	// Test basic joining
	result := SafeJoin("path", "to", "file")
	expected := filepath.Join("path", "to", "file")
	assert.Equal(suite.T(), expected, result)

	// Test with relative paths and normalization
	result = SafeJoin("path", "..", "to", "file")
	expected = filepath.Join("path", "..", "to", "file")
	expected = filepath.Clean(expected)
	assert.Equal(suite.T(), expected, result)

	// Test with empty elements
	result = SafeJoin("path", "", "to", "file")
	expected = filepath.Join("path", "", "to", "file")
	assert.Equal(suite.T(), expected, result)

	// Test single element
	result = SafeJoin("single")
	assert.Equal(suite.T(), "single", result)

	// Test no elements (should handle gracefully)
	result = SafeJoin()
	assert.Equal(suite.T(), ".", result)
}

// TestMakeDirs tests directory creation with cross-platform permissions
func (suite *FsutilTestSuite) TestMakeDirs() {
	testPath := filepath.Join(suite.tempDir, "test", "nested", "dirs")

	// Create nested directories
	err := MakeDirs(testPath)
	assert.NoError(suite.T(), err)

	// Verify directories exist
	_, err = os.Stat(testPath)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), isDir(testPath))

	// Verify parent directories also exist
	parentPath := filepath.Join(suite.tempDir, "test", "nested")
	_, err = os.Stat(parentPath)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), isDir(parentPath))
}

// TestMakeDirs_Existing tests creating directories that already exist
func (suite *FsutilTestSuite) TestMakeDirs_Existing() {
	testPath := filepath.Join(suite.tempDir, "existing")

	// Create directory first
	err := os.MkdirAll(testPath, 0755)
	assert.NoError(suite.T(), err)

	// Try to create again - should not error
	err = MakeDirs(testPath)
	assert.NoError(suite.T(), err)
}

// TestOpenFile_Write tests opening a file for writing
func (suite *FsutilTestSuite) TestOpenFile_Write() {
	testFile := filepath.Join(suite.tempDir, "write_test.txt")

	file, err := OpenFile(testFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), file)
	defer file.Close()

	// Write some data
	_, err = file.WriteString("test content")
	assert.NoError(suite.T(), err)

	// Close and verify file exists with content
	file.Close()
	content, err := os.ReadFile(testFile)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "test content", string(content))
}

// TestOpenFile_Read tests opening a file for reading
func (suite *FsutilTestSuite) TestOpenFile_Read() {
	testFile := filepath.Join(suite.tempDir, "read_test.txt")
	testContent := "test read content"

	// Create file with content
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	assert.NoError(suite.T(), err)

	// Open for reading
	file, err := OpenFile(testFile, os.O_RDONLY, 0)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), file)
	defer file.Close()

	// Read content
	content := make([]byte, len(testContent))
	_, err = file.Read(content)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), testContent, string(content))
}

// TestReadFile tests the ReadFile convenience function
func (suite *FsutilTestSuite) TestReadFile() {
	testFile := filepath.Join(suite.tempDir, "readfile_test.txt")
	testContent := "readfile test content"

	// Create file with content
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	assert.NoError(suite.T(), err)

	// Use ReadFile
	file, err := ReadFile(testFile)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), file)
	defer file.Close()

	// Verify it's opened for reading
	content := make([]byte, len(testContent))
	_, err = file.Read(content)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), testContent, string(content))
}

// TestWriteFile tests the WriteFile convenience function
func (suite *FsutilTestSuite) TestWriteFile() {
	testFile := filepath.Join(suite.tempDir, "writefile_test.txt")

	file, err := WriteFile(testFile)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), file)
	defer file.Close()

	// Write content
	testContent := "writefile test content"
	_, err = file.WriteString(testContent)
	assert.NoError(suite.T(), err)
	file.Close()

	// Verify file was created with correct content
	content, err := os.ReadFile(testFile)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), testContent, string(content))
}

// TestAppendFile tests the AppendFile convenience function
func (suite *FsutilTestSuite) TestAppendFile() {
	testFile := filepath.Join(suite.tempDir, "appendfile_test.txt")
	initialContent := "initial content"

	// Create file with initial content
	err := os.WriteFile(testFile, []byte(initialContent), 0644)
	assert.NoError(suite.T(), err)

	// Append to file
	file, err := AppendFile(testFile)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), file)
	defer file.Close()

	// Append content
	appendContent := " appended content"
	_, err = file.WriteString(appendContent)
	assert.NoError(suite.T(), err)
	file.Close()

	// Verify content was appended
	content, err := os.ReadFile(testFile)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), initialContent+appendContent, string(content))
}

// TestReadTxtFile tests reading text files with line filtering
func (suite *FsutilTestSuite) TestReadTxtFile() {
	testFile := filepath.Join(suite.tempDir, "textfile_test.txt")
	fileContent := `line 1
empty line

line 2
  spaced line
line 3
`

	// Create test file
	err := os.WriteFile(testFile, []byte(fileContent), 0644)
	assert.NoError(suite.T(), err)

	// Read text file
	lines, err := ReadTxtFile(testFile)
	assert.NoError(suite.T(), err)

	// Should filter out empty lines and trim whitespace
	// "empty line" is not empty (it contains text), so it should be included
	expected := []string{"line 1", "empty line", "line 2", "spaced line", "line 3"}
	assert.Equal(suite.T(), expected, lines)
}

// TestReadTxtFile_EmptyFile tests reading an empty file
func (suite *FsutilTestSuite) TestReadTxtFile_EmptyFile() {
	testFile := filepath.Join(suite.tempDir, "empty_test.txt")

	// Create empty file
	err := os.WriteFile(testFile, []byte(""), 0644)
	assert.NoError(suite.T(), err)

	lines, err := ReadTxtFile(testFile)
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), lines)
}

// TestReadTxtFile_OnlyEmptyLines tests file with only empty lines
func (suite *FsutilTestSuite) TestReadTxtFile_OnlyEmptyLines() {
	testFile := filepath.Join(suite.tempDir, "only_empty_test.txt")
	fileContent := `

`

	// Create file with only empty lines
	err := os.WriteFile(testFile, []byte(fileContent), 0644)
	assert.NoError(suite.T(), err)

	lines, err := ReadTxtFile(testFile)
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), lines)
}

// TestReadTxtFile_NonExistent tests reading a non-existent file
func (suite *FsutilTestSuite) TestReadTxtFile_NonExistent() {
	nonExistentFile := filepath.Join(suite.tempDir, "nonexistent.txt")

	lines, err := ReadTxtFile(nonExistentFile)
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), lines)
}

// TestReadTxtFile_WhitespaceOnly tests file with only whitespace
func (suite *FsutilTestSuite) TestReadTxtFile_WhitespaceOnly() {
	testFile := filepath.Join(suite.tempDir, "whitespace_test.txt")
	fileContent := "   \t   \n\t\t\n  \t  "

	// Create file with only whitespace
	err := os.WriteFile(testFile, []byte(fileContent), 0644)
	assert.NoError(suite.T(), err)

	lines, err := ReadTxtFile(testFile)
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), lines)
}

// TestOpenFile_DefaultPerms tests that OpenFile uses default permissions when perm is 0
func (suite *FsutilTestSuite) TestOpenFile_DefaultPerms() {
	testFile := filepath.Join(suite.tempDir, "default_perms_test.txt")

	file, err := OpenFile(testFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), file)
	file.Close()

	// Verify file was created
	_, err = os.Stat(testFile)
	assert.NoError(suite.T(), err)
}

// TestOpenFile_CustomPerms tests OpenFile with custom permissions
func (suite *FsutilTestSuite) TestOpenFile_CustomPerms() {
	testFile := filepath.Join(suite.tempDir, "custom_perms_test.txt")

	customPerms := os.FileMode(0600)
	file, err := OpenFile(testFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, customPerms)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), file)
	file.Close()

	// Verify file was created with custom permissions
	info, err := os.Stat(testFile)
	assert.NoError(suite.T(), err)
	// Note: On Windows, permissions might not be exactly as set due to platform limitations
	if runtime.GOOS != "windows" {
		assert.Equal(suite.T(), customPerms, info.Mode().Perm())
	}
}

// Helper function to check if path is a directory
func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// Run the test suite
func TestFsutilTestSuite(t *testing.T) {
	suite.Run(t, new(FsutilTestSuite))
}
