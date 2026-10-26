// Package configedit changes the configuration file the way a person
// would: it edits the YAML tree, so comments and the order of keys stay,
// keeps references between sections consistent (renaming a code renames
// it everywhere) and validates the result before it is saved.
package configedit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"

	"github.com/just-tomdo/housenka/internal/atomicfile"
	"github.com/just-tomdo/housenka/internal/config"
)

// ErrConflict means the file changed since it was loaded.
var ErrConflict = errors.New("config se mezitím změnil (třeba v editoru nebo v jiném okně) – načti stránku znovu")

// Doc is a configuration file being edited.
type Doc struct {
	root   *yaml.Node // document node
	source []byte     // the data the document was parsed from
	hash   string     // of source
}

// Hash identifies file contents; forms carry it to detect concurrent changes.
func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Parse reads a configuration from YAML. It does not validate it.
func Parse(data []byte) (*Doc, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("chyba syntaxe YAML: %w", err)
	}
	if root.Kind == 0 { // empty file
		root = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("config musí být mapa klíčů (gitlab_hosts, bm, projects …)")
	}
	return &Doc{root: &root, source: data, hash: Hash(data)}, nil
}

// Load reads the configuration file at path.
func Load(path string) (*Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Hash returns the hash of the data the document was loaded from.
func (d *Doc) Hash() string { return d.hash }

// Bytes renders the document as YAML.
func (d *Doc) Bytes() ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(d.root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return pretty(b.Bytes(), d.source), nil
}

// Config decodes and validates the edited configuration.
func (d *Doc) Config() (*config.Config, error) {
	data, err := d.Bytes()
	if err != nil {
		return nil, err
	}
	return config.Parse(data)
}

// Save validates the document and writes it to path, unless the file
// changed since the document was loaded (ErrConflict). The previous
// contents are kept in path + ".bak".
func (d *Doc) Save(path string) error {
	data, err := d.Bytes()
	if err != nil {
		return err
	}
	if _, err := config.Parse(data); err != nil {
		return err
	}
	return WriteFile(path, data, d.hash)
}

// WriteFile replaces the file at path with data if its current contents
// still have expectedHash. The previous contents are kept in path + ".bak"
// with the same permissions; the file is replaced atomically, a symlink
// stays a symlink and the permissions of the file are kept.
func WriteFile(path string, data []byte, expectedHash string) error {
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	case Hash(old) != expectedHash:
		return ErrConflict
	default:
		perm := os.FileMode(0o644)
		if fi, err := os.Stat(path); err == nil {
			perm = fi.Mode().Perm()
		}
		if err := os.Remove(path + ".bak"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := atomicfile.Write(path+".bak", old, perm); err != nil {
			return err
		}
	}
	return atomicfile.Write(path, data, 0o644)
}
