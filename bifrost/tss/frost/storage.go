package frost

import (
	"fmt"
	"os"
	"path/filepath"
)

type KeyShareStore interface {
	Load(poolPubKey string) ([]byte, error)
	Save(poolPubKey string, keyShare []byte) error
}

type FileKeyShareStore struct {
	dir string
}

func NewFileKeyShareStore(dir string) (*FileKeyShareStore, error) {
	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyshare dir: %w", err)
	}
	return &FileKeyShareStore{dir: dir}, nil
}

func (s *FileKeyShareStore) Load(poolPubKey string) ([]byte, error) {
	p := filepath.Join(s.dir, poolPubKey+".keyshare")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("failed to load keyshare for %s: %w", poolPubKey, err)
	}
	return data, nil
}

func (s *FileKeyShareStore) Save(poolPubKey string, keyShare []byte) error {
	p := filepath.Join(s.dir, poolPubKey+".keyshare")
	err := os.WriteFile(p, keyShare, 0o600)
	if err != nil {
		return fmt.Errorf("failed to save keyshare for %s: %w", poolPubKey, err)
	}
	return nil
}
