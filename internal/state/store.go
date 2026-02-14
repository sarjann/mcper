package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/sarjann/mcper/internal/fsutil"
	"github.com/sarjann/mcper/internal/model"
	"github.com/sarjann/mcper/internal/paths"
)

type Store struct {
	path string
}

func NewStore() (*Store, error) {
	path, err := paths.StatePath()
	if err != nil {
		return nil, err
	}
	return &Store{path: path}, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Load() (model.State, error) {
	if err := paths.EnsureDirDirOf(s.path); err != nil {
		return model.State{}, err
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			st := model.NewDefaultState()
			if err := s.Save(st); err != nil {
				return model.State{}, err
			}
			return st, nil
		}
		return model.State{}, fmt.Errorf("read state file: %w", err)
	}

	var st model.State
	if err := json.Unmarshal(data, &st); err != nil {
		return model.State{}, fmt.Errorf("decode state file: %w", err)
	}

	if st.Version == 0 {
		st.Version = model.StateVersion
	}
	if st.Taps == nil {
		st.Taps = map[string]model.TapConfig{}
	}
	if st.Installed == nil {
		st.Installed = map[string]model.InstalledPackage{}
	}
	if st.TrustedDirectSources == nil {
		st.TrustedDirectSources = map[string]model.TrustDecision{}
	}
	if _, ok := st.Taps[model.DefaultTapName]; !ok {
		def := model.NewDefaultState()
		st.Taps[model.DefaultTapName] = def.Taps[model.DefaultTapName]
	}

	return st, nil
}

func (s *Store) Save(st model.State) error {
	if st.Version == 0 {
		st.Version = model.StateVersion
	}
	if st.Taps == nil {
		st.Taps = map[string]model.TapConfig{}
	}
	if st.Installed == nil {
		st.Installed = map[string]model.InstalledPackage{}
	}
	if st.TrustedDirectSources == nil {
		st.TrustedDirectSources = map[string]model.TrustDecision{}
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state file: %w", err)
	}
	if err := paths.EnsureDirDirOf(s.path); err != nil {
		return err
	}
	if err := fsutil.AtomicWriteFile(s.path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	return nil
}

func (s *Store) MustLoad() model.State {
	st, err := s.Load()
	if err != nil {
		panic(err)
	}
	return st
}

func ErrNotFound(kind, name string) error {
	return fmt.Errorf("%s %q not found", kind, name)
}

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, os.ErrNotExist)
}
