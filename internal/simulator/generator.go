package simulator

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// GenerateFakeFile creates a mock log file of the specified size.
func GenerateFakeFile(workspace, serviceName string, sizeKB int) (string, error) {
	dir := filepath.Join(workspace, serviceName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	fileName := fmt.Sprintf("%s_%d.log", serviceName, time.Now().UnixNano())
	filePath := filepath.Join(dir, fileName)

	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Write pseudo JSON logs until target size
	line := []byte(fmt.Sprintf(`{"time":"%s","level":"INFO","service":"%s","msg":"dummy log data generating for simulation purposes"}`+"\n", time.Now().Format(time.RFC3339), serviceName))
	lineSize := len(line)
	targetBytes := sizeKB * 1024

	// Write quickly in chunks
	for written := 0; written < targetBytes; written += lineSize {
		if _, err := file.Write(line); err != nil {
			return "", err
		}
	}

	return filePath, nil
}
