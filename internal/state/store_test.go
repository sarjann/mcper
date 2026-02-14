package state

import (
	"path/filepath"
	"testing"

	"github.com/sarjann/mcper/internal/model"
)

func TestStoreLoadInitializesDefaultState(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	st, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if st.Version != model.StateVersion {
		t.Fatalf("expected state version %d, got %d", model.StateVersion, st.Version)
	}
	if _, ok := st.Taps[model.DefaultTapName]; !ok {
		t.Fatalf("expected default tap %q", model.DefaultTapName)
	}

	expectedPath := filepath.Join(tmp, "mcper", "state.json")
	if s.Path() != expectedPath {
		t.Fatalf("expected state path %s, got %s", expectedPath, s.Path())
	}
}
