package storage

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"tardis/internal/queue"
)

type GoogleDriveAdapter struct {
	srv *drive.Service
	db  *queue.DB
}

func NewGoogleDriveAdapter(ctx context.Context, credentialsPath string, db *queue.DB) (*GoogleDriveAdapter, error) {
	b, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file: %v", err)
	}

	srv, err := drive.NewService(ctx, option.WithCredentialsJSON(b))
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve Drive client: %v", err)
	}

	return &GoogleDriveAdapter{
		srv: srv,
		db:  db,
	}, nil
}

func (g *GoogleDriveAdapter) Upload(ctx context.Context, localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer f.Close()

	parentPath := filepath.Dir(remotePath)
	fileName := filepath.Base(remotePath)

	parentID, err := g.GetPathID(ctx, parentPath)
	if err != nil {
		return fmt.Errorf("failed to get parent ID: %w", err)
	}

	driveFile := &drive.File{
		Name:    fileName,
		Parents: []string{parentID},
	}

	// For resumable chunked upload, the Drive API library handles it automatically 
	// when we use .Media() with a file reader.
	_, err = g.srv.Files.Create(driveFile).Media(f).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	return nil
}

func (g *GoogleDriveAdapter) Download(ctx context.Context, remotePath, localPath string) error {
	fileID, err := g.GetPathID(ctx, remotePath)
	if err != nil {
		return fmt.Errorf("failed to get file ID: %w", err)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer out.Close()

	// Implement chunked download using HTTP Range
	const chunkSize = 10 * 1024 * 1024 // 10MB chunk
	var offset int64 = 0

	for {
		req := g.srv.Files.Get(fileID)
		req.Header().Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+chunkSize-1))
		
		res, err := req.Download()
		if err != nil {
			// If we get a 416 Range Not Satisfiable, we reached the end
			if gErr, ok := err.(*googleapi.Error); ok && gErr.Code == http.StatusRequestedRangeNotSatisfiable {
				break
			}
			return fmt.Errorf("failed to download chunk at offset %d: %w", offset, err)
		}

		n, err := io.Copy(out, res.Body)
		res.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to write chunk to disk: %w", err)
		}

		if n == 0 {
			break
		}
		offset += n
		
		// If we downloaded less than chunkSize, we've reached the end of the file
		if n < chunkSize {
			break
		}
	}

	return nil
}

func (g *GoogleDriveAdapter) GetPathID(ctx context.Context, path string) (string, error) {
	if path == "/" || path == "" || path == "." {
		return "root", nil
	}

	// Clean and standardize path
	path = filepath.Clean(path)
	
	// Check Cache
	var objectID string
	err := g.db.QueryRowContext(ctx, "SELECT object_id FROM path_cache WHERE path = ?", path).Scan(&objectID)
	if err == nil {
		return objectID, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("db error checking path cache: %w", err)
	}

	// Resolve path via API
	parts := strings.Split(strings.Trim(path, "/"), "/")
	currentID := "root"
	currentPath := ""

	for _, part := range parts {
		if part == "" {
			continue
		}
		currentPath += "/" + part
		
		// Search for folder with currentID as parent and name as part
		query := fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false", currentID, part)
		fileList, err := g.srv.Files.List().Q(query).Fields("files(id, name)").Context(ctx).Do()
		if err != nil {
			return "", fmt.Errorf("failed to list files: %w", err)
		}

		if len(fileList.Files) == 0 {
			return "", fmt.Errorf("path not found: %s", currentPath)
		}

		currentID = fileList.Files[0].Id
		
		// Save to cache
		_, _ = g.db.ExecContext(ctx, "INSERT OR REPLACE INTO path_cache (path, object_id) VALUES (?, ?)", currentPath, currentID)
	}

	return currentID, nil
}

