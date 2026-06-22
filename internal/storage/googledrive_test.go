package storage

import (
	"context"
	"testing"
)

func TestGoogleDriveAdapter_ResolveRemotePath(t *testing.T) {
	adapter := &GoogleDriveAdapter{
		rootDir: "my-tardis-root",
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"", "/my-tardis-root"},
		{"/", "/my-tardis-root"},
		{"file.txt", "/my-tardis-root/file.txt"},
		{"/file.txt", "/my-tardis-root/file.txt"},
		{"folder/file.txt", "/my-tardis-root/folder/file.txt"},
		{"/folder/file.txt", "/my-tardis-root/folder/file.txt"},
		{"../outside.txt", "/my-tardis-root/outside.txt"}, // filepath.Clean/Join prevents escaping root
		{"folder/sub/../../file.txt", "/my-tardis-root/file.txt"},
	}

	for _, tc := range tests {
		actual := adapter.resolveRemotePath(tc.input)
		if actual != tc.expected {
			t.Errorf("resolveRemotePath(%q) = %q; expected %q", tc.input, actual, tc.expected)
		}
	}
}

func TestNewGoogleDriveAdapter_NonExistentCreds(t *testing.T) {
	ctx := context.Background()
	_, err := NewGoogleDriveAdapter(ctx, "non_existent_creds.json", "token.json", "root", nil)
	if err == nil {
		t.Fatal("expected error when creating adapter with non-existent credentials, got nil")
	}
}
