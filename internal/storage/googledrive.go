package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"hippodrop/internal/queue"
)

type GoogleDriveAdapter struct {
	srv     *drive.Service
	db      *queue.DB
	rootDir string
}

func NewGoogleDriveAdapter(ctx context.Context, credentialsPath string, tokenPath string, rootDir string, db *queue.DB) (*GoogleDriveAdapter, error) {
	b, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveFileScope, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secret file to config: %v", err)
	}

	tokFile, err := os.Open(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("unable to open token file: %v", err)
	}
	defer tokFile.Close()

	tok := &oauth2.Token{}
	if err = json.NewDecoder(tokFile).Decode(tok); err != nil {
		return nil, fmt.Errorf("unable to decode token: %v", err)
	}

	client := config.Client(ctx, tok)
	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve Drive client: %v", err)
	}

	if rootDir == "" {
		rootDir = "hippodrop"
	}

	return &GoogleDriveAdapter{
		srv:     srv,
		db:      db,
		rootDir: rootDir,
	}, nil
}

func (g *GoogleDriveAdapter) resolveRemotePath(remotePath string) string {
	cleaned := filepath.Clean(remotePath)
	// Prevent directory traversal (escaping rootDir) by stripping leading "../" or ".."
	for strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		if cleaned == ".." {
			cleaned = "."
			break
		}
		cleaned = cleaned[3:]
	}
	cleaned = strings.TrimPrefix(cleaned, "/")

	joined := filepath.Join("/", g.rootDir, cleaned)
	return filepath.Clean(joined)
}

func (g *GoogleDriveAdapter) Upload(ctx context.Context, localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer f.Close()

	resolvedPath := g.resolveRemotePath(remotePath)
	parentPath := filepath.Dir(resolvedPath)
	fileName := filepath.Base(resolvedPath)

	parentID, err := g.GetPathID(ctx, parentPath, true)
	if err != nil {
		return fmt.Errorf("failed to get parent ID: %w", err)
	}

	driveFile := &drive.File{
		Name:    fileName,
		Parents: []string{parentID},
	}

	// For resumable chunked upload, the Drive API library handles it automatically 
	// when we use .Media() with a file reader.
	_, err = g.srv.Files.Create(driveFile).Media(f).Context(ctx).SupportsAllDrives(true).Do()
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	return nil
}

func (g *GoogleDriveAdapter) Download(ctx context.Context, remotePath, localPath string) error {
	resolvedPath := g.resolveRemotePath(remotePath)
	fileID, err := g.GetPathID(ctx, resolvedPath, false)
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
		req := g.srv.Files.Get(fileID).SupportsAllDrives(true)
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

func (g *GoogleDriveAdapter) GetPathID(ctx context.Context, path string, createIfMissing bool) (string, error) {
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
		
		var query string
		escapedPart := escapeDriveQuery(part)
		if currentID == "root" {
			// First folder might be shared with the SA, so we search globally
			query = fmt.Sprintf("name = '%s' and mimeType = 'application/vnd.google-apps.folder' and trashed = false", escapedPart)
		} else {
			// Search inside the parent folder
			query = fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false", escapeDriveQuery(currentID), escapedPart)
		}
		
		fileList, err := g.srv.Files.List().Q(query).Fields("files(id, name)").SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Context(ctx).Do()
		if err != nil {
			return "", fmt.Errorf("failed to list files: %w", err)
		}

		if len(fileList.Files) == 0 {
			if createIfMissing {
				newFolder := &drive.File{
					Name:     part,
					MimeType: "application/vnd.google-apps.folder",
					Parents:  []string{currentID},
				}
				created, err := g.srv.Files.Create(newFolder).Fields("id").SupportsAllDrives(true).Context(ctx).Do()
				if err != nil {
					return "", fmt.Errorf("failed to create missing folder %s: %w", currentPath, err)
				}
				currentID = created.Id
			} else {
				return "", fmt.Errorf("path not found: %s", currentPath)
			}
		} else {
			currentID = fileList.Files[0].Id
		}
		
		// Save to cache
		_, _ = g.db.ExecContext(ctx, "INSERT OR REPLACE INTO path_cache (path, object_id) VALUES (?, ?)", currentPath, currentID)
	}

	return currentID, nil
}

// escapeDriveQuery escapes single quotes in values used in Google Drive API queries.
func escapeDriveQuery(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}

