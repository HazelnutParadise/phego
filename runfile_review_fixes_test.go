package phego

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

type reviewNonregularFS struct {
	mode          fs.FileMode
	reads, closes int
}
type reviewNonregularFile struct {
	owner *reviewNonregularFS
	fs.File
}

func (s *reviewNonregularFS) Open(string) (fs.File, error) {
	f, err := (fstest.MapFS{"file": &fstest.MapFile{Mode: s.mode, Data: []byte("<?php return 1;")}}).Open("file")
	return &reviewNonregularFile{owner: s, File: f}, err
}
func (f *reviewNonregularFile) Read(b []byte) (int, error) { f.owner.reads++; return f.File.Read(b) }
func (f *reviewNonregularFile) Close() error               { f.owner.closes++; return f.File.Close() }
func TestReviewFixRunFileRejectsNonregularBeforeRead(t *testing.T) {
	for _, mode := range []fs.FileMode{fs.ModeNamedPipe, fs.ModeDevice, fs.ModeSocket, fs.ModeDir} {
		for _, legacy := range []bool{false, true} {
			s := &reviewNonregularFS{mode: mode}
			cfg := Config{MaxActive: 1}
			name := "/app/file"
			if legacy {
				cfg.Files = s
				name = "file"
			} else {
				cfg.Policy.Files = []FileGrant{{Path: "/app", FS: s}}
			}
			e := newPolicyEngine(t, cfg)
			_, err := e.RunFile(context.Background(), name)
			if err == nil {
				t.Error("nonregular source accepted")
			}
			if s.reads != 0 || s.closes != 1 {
				t.Errorf("mode=%v legacy=%v reads=%d closes=%d error=%v", mode, legacy, s.reads, s.closes, err)
			}
			if e.policy.admission.Active() != 0 {
				t.Error("admission slot leaked")
			}
			release, err := e.admit(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			release()
		}
	}
}
func TestReviewFixPolicyErrorTargetIsPHPPath(t *testing.T) {
	e := newPolicyEngine(t, Config{Policy: Policy{Files: []FileGrant{{Path: "/app", Root: t.TempDir()}}}})
	_, err := e.RunFile(context.Background(), "/outside/file.php")
	var pe *PolicyError
	if !errors.As(err, &pe) || pe.Target != "/outside/file.php" {
		t.Fatalf("got %v", err)
	}
}
