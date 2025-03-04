package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
)

type DirectoryEntry struct {
	File
	Name string
}

type Directory interface {
	File

	GetChild(name string) (DirectoryEntry, error)
	Readdir() ([]DirectoryEntry, error)
}

func getMutable(dir Directory) MutableDirectory {
	if mut, ok := dir.(MutableDirectory); ok {
		return mut
	} else if mut, ok := dir.(*StarDirectory); ok {
		return getMutable(mut.Directory)
	} else {
		return nil
	}
}

func Exists(dir Directory, p string) bool {
	_, err := OpenPath(dir, p)
	return err == nil
}

func resolveDirectory(root Directory, file File, name string) (Directory, error) {
	if dir, ok := file.(Directory); ok {
		return dir, nil
	}

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	switch info.Kind() {
	case TypeSymlink:
		target, err := GetLinkName(file)
		if err != nil {
			return nil, err
		}

		currentDir := path.Dir(name)

		newTarget := path.Join(currentDir, target)

		ent, err := OpenPath(root, newTarget)
		if err != nil {
			return nil, err
		}

		return resolveDirectory(root, ent.File, newTarget)
	default:
		return nil, fmt.Errorf("OpenPath(%s): child %T is not a directory (kind=%s)", name, file, info.Kind())
	}
}

func OpenPath(dir Directory, p string) (DirectoryEntry, error) {
	p = strings.TrimPrefix(p, "/")

	tokens := strings.Split(path.Clean(p), "/")

	var currentDir = dir

	for i, token := range tokens[:len(tokens)-1] {
		child, err := currentDir.GetChild(token)
		if err != nil {
			return DirectoryEntry{}, err
		}

		childDir, err := resolveDirectory(dir, child.File, path.Join(tokens[:i+1]...))
		if err != nil {
			return DirectoryEntry{}, err
		}

		currentDir = childDir
	}

	dirname := tokens[len(tokens)-1]

	if dirname == "." {
		return DirectoryEntry{
			File: currentDir,
			Name: ".",
		}, nil
	}

	return currentDir.GetChild(dirname)
}

func Mkdir(dir Directory, p string) (MutableDirectory, error) {
	p = strings.TrimPrefix(p, "/")

	tokens := strings.Split(path.Clean(p), "/")

	var currentDir = dir

	for i, token := range tokens[:len(tokens)-1] {
		child, err := currentDir.GetChild(token)
		if err == fs.ErrNotExist {
			if mut := getMutable(currentDir); mut != nil {
				newChild, err := mut.Mkdir(token)
				if err != nil {
					return nil, err
				}

				child = DirectoryEntry{File: newChild, Name: token}
			} else {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}

		childDir, err := resolveDirectory(dir, child.File, path.Join(tokens[:i+1]...))
		if err != nil {
			return nil, err
		}

		currentDir = childDir
	}

	mut := getMutable(currentDir)
	if mut == nil {
		return nil, fmt.Errorf("directory %T is not mutable", currentDir)
	}

	dirname := tokens[len(tokens)-1]

	if dirname == "." {
		return mut, nil
	}

	return mut.Mkdir(dirname)
}

func CreateChild(dir Directory, p string, f File) error {
	p = strings.TrimPrefix(p, "/")

	tokens := strings.Split(path.Clean(p), "/")

	var currentDir = dir

	for i, token := range tokens[:len(tokens)-1] {
		child, err := currentDir.GetChild(token)
		if err == fs.ErrNotExist {
			if mut := getMutable(currentDir); mut != nil {
				newChild, err := mut.Mkdir(token)
				if err != nil {
					return err
				}

				child = DirectoryEntry{File: newChild, Name: token}
			} else {
				return err
			}
		} else if err != nil {
			return err
		}

		childDir, err := resolveDirectory(dir, child.File, path.Join(tokens[:i+1]...))
		if err != nil {
			return err
		}

		currentDir = childDir
	}

	mut := getMutable(currentDir)
	if mut == nil {
		return fmt.Errorf("directory %T is not mutable", currentDir)
	}

	return mut.Create(tokens[len(tokens)-1], f)
}

func GetTotalSize(dir Directory) (int64, error) {
	ents, err := dir.Readdir()
	if err != nil {
		return -1, err
	}

	var total int64 = 0

	for _, child := range ents {
		info, err := child.Stat()
		if err != nil {
			return -1, err
		}

		switch info.Kind() {
		case TypeRegular:
			total += info.Size()
		case TypeDirectory:
			dir, ok := child.File.(Directory)
			if !ok {
				return -1, fmt.Errorf("child is not a directory %T", child.File)
			}

			childTotal, err := GetTotalSize(dir)
			if err != nil {
				return -1, err
			}

			total += childTotal
		default:
			continue
		}
	}

	return total, nil
}

type MutableDirectory interface {
	Directory
	MutableFile

	Mkdir(name string) (MutableDirectory, error)
	Create(name string, f File) error
	Unlink(name string) error
}

type memoryDirectory struct {
	*memoryFile

	names   []string
	entries map[string]File
}

// IsDir implements FileInfo.
func (m *memoryDirectory) IsDir() bool {
	return true
}

// Sys implements FileInfo.
func (m *memoryDirectory) Sys() any {
	return m
}

// Unlink implements MutableDirectory.
func (m *memoryDirectory) Unlink(name string) error {
	if path.Base(name) != name {
		return fmt.Errorf("MutableDirectory methods can not handle paths: %s", name)
	}

	delete(m.entries, name)

	return nil
}

// Create implements MutableDirectory.
func (m *memoryDirectory) Create(name string, f File) error {
	if name == "" || name == "." {
		return fmt.Errorf("invalid name specified for child: %s", name)
	}

	if path.Base(name) != name {
		return fmt.Errorf("MutableDirectory methods can not handle paths: %s", name)
	}

	if _, exists := m.entries[name]; exists {
		return nil
	}

	m.names = append(m.names, name)
	m.entries[name] = f

	return nil
}

// GetChild implements MutableDirectory.
func (m *memoryDirectory) GetChild(name string) (DirectoryEntry, error) {
	if name == "" || name == "." {
		return DirectoryEntry{File: m}, nil
	}

	if path.Base(name) != name {
		return DirectoryEntry{}, fmt.Errorf("MutableDirectory methods can not handle paths: %s", name)
	}

	child, ok := m.entries[name]
	if !ok {
		return DirectoryEntry{}, fs.ErrNotExist
	}

	return DirectoryEntry{File: child, Name: name}, nil
}

// Mkdir implements MutableDirectory.
func (m *memoryDirectory) Mkdir(name string) (MutableDirectory, error) {
	if name == "" || name == "." {
		return nil, fmt.Errorf("invalid name specified for child: %s", name)
	}

	if path.Base(name) != name {
		return nil, fmt.Errorf("MutableDirectory methods can not handle paths: %s", name)
	}

	if ent, exists := m.entries[name]; exists {
		if dir, ok := ent.(Directory); ok {
			mut := getMutable(dir)
			if mut != nil {
				return mut, nil
			} else {
				return nil, fmt.Errorf("child is not mutable: %T", dir)
			}
		} else {
			return nil, fmt.Errorf("entry is not a directory: %T", ent)
		}
	}

	child := NewMemoryDirectory()

	if err := m.Create(name, child); err != nil {
		return nil, err
	}

	return child, nil
}

// Open implements MutableDirectory.
func (m *memoryDirectory) Open() (FileHandle, error) {
	return nil, fs.ErrInvalid
}

// Overwrite implements MutableDirectory.
func (m *memoryDirectory) Overwrite(contents []byte) error {
	return fs.ErrInvalid
}

// Readdir implements MutableDirectory.
func (m *memoryDirectory) Readdir() ([]DirectoryEntry, error) {
	var ret []DirectoryEntry

	for _, name := range m.names {
		file := m.entries[name]
		ret = append(ret, DirectoryEntry{File: file, Name: name})
	}

	return ret, nil
}

// Stat implements MutableDirectory.
func (m *memoryDirectory) Stat() (FileInfo, error) {
	return m, nil
}

var (
	_ MutableDirectory = &memoryDirectory{}
)

func NewMemoryDirectory() MutableDirectory {
	f := NewMemoryFile(TypeDirectory).(*memoryFile)
	f.mode = fs.ModeDir | fs.FileMode(0755)
	return &memoryDirectory{
		memoryFile: f,
		entries:    make(map[string]File),
	}
}

func ExtractEntry(ent Entry, dir MutableDirectory) error {
	switch ent.Typeflag() {
	case TypeDirectory:
		name := strings.TrimSuffix(ent.Name(), "/")
		name = strings.TrimPrefix(name, "./")

		child, err := Mkdir(dir, name)
		if errors.Is(err, os.ErrExist) {
			return nil
		} else if err != nil {
			return err
		}

		if err := child.Chmod(ent.Mode()); err != nil {
			return err
		}

		if err := child.Chown(ent.Uid(), ent.Gid()); err != nil {
			return err
		}

		if err := child.Chtimes(ent.ModTime()); err != nil {
			return err
		}

		return nil
	case TypeRegular:
		return CreateChild(dir, ent.Name(), ent)
	case TypeSymlink:
		return CreateChild(dir, ent.Name(), ent)
	case TypeLink:
		return CreateChild(dir, ent.Name(), ent)
	default:
		return fmt.Errorf("unknown Entry type: %s", ent.Typeflag())
	}
}
