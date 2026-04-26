package scanner

import (
	"testing"
)

func TestClassify(t *testing.T) {
	inodes := map[uint64]LibraryRef{
		100: {Source: "sonarr", Title: "Breaking Bad"},
	}
	libPaths := map[string]LibraryRef{
		"/downloads/movies/Dune.mkv": {Source: "radarr", Title: "Dune"},
	}
	torrentPaths := map[string]TorrentRef{
		"/downloads/movies/Dune.mkv":      {Name: "Dune"},
		"/downloads/seeding/somefile.rar": {Name: "some.torrent"},
	}

	c := NewClassifier(inodes, libPaths, torrentPaths)

	tests := []struct {
		name    string
		path    string
		inode   uint64
		wantCat Category
		wantLib bool
		wantTor bool
	}{
		{
			name:    "library+seeding via inode+path",
			path:    "/downloads/movies/Dune.mkv",
			inode:   0,
			wantCat: CategoryLibrarySeeding,
			wantLib: true,
			wantTor: true,
		},
		{
			name:    "library-only via inode (path not in torrent set)",
			path:    "/downloads/tv/bb/s01e01.mkv",
			inode:   100,
			wantCat: CategoryLibraryOnly,
			wantLib: true,
			wantTor: false,
		},
		{
			name:    "library-only via path",
			path:    "/downloads/movies/Dune.mkv",
			inode:   999,                    // unknown inode
			wantCat: CategoryLibrarySeeding, // path matches torrent too
			wantLib: true,
			wantTor: true,
		},
		{
			name:    "seeding-only",
			path:    "/downloads/seeding/somefile.rar",
			inode:   0,
			wantCat: CategorySeedingOnly,
			wantLib: false,
			wantTor: true,
		},
		{
			name:    "orphan",
			path:    "/downloads/old/junk.zip",
			inode:   0,
			wantCat: CategoryOrphan,
			wantLib: false,
			wantTor: false,
		},
		{
			name:    "library-only via inode no torrent",
			path:    "/downloads/tv/bb/renamed.mkv",
			inode:   100,
			wantCat: CategoryLibraryOnly,
			wantLib: true,
			wantTor: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Classify(tt.path, tt.inode)
			if got.Category != tt.wantCat {
				t.Errorf("Classify(%q, %d).Category = %q, want %q",
					tt.path, tt.inode, got.Category, tt.wantCat)
			}
			if (got.LibraryRef != nil) != tt.wantLib {
				t.Errorf("Classify(%q, %d).LibraryRef presence = %v, want %v",
					tt.path, tt.inode, got.LibraryRef != nil, tt.wantLib)
			}
			if (got.TorrentRef != nil) != tt.wantTor {
				t.Errorf("Classify(%q, %d).TorrentRef presence = %v, want %v",
					tt.path, tt.inode, got.TorrentRef != nil, tt.wantTor)
			}
		})
	}
}

func TestDetermineCategory(t *testing.T) {
	ref := &LibraryRef{Source: "sonarr", Title: "Test"}
	tor := &TorrentRef{Name: "test.torrent"}

	if got := determineCategory(ref, tor); got != CategoryLibrarySeeding {
		t.Errorf("want library_seeding, got %s", got)
	}
	if got := determineCategory(ref, nil); got != CategoryLibraryOnly {
		t.Errorf("want library_only, got %s", got)
	}
	if got := determineCategory(nil, tor); got != CategorySeedingOnly {
		t.Errorf("want seeding_only, got %s", got)
	}
	if got := determineCategory(nil, nil); got != CategoryOrphan {
		t.Errorf("want orphan, got %s", got)
	}
}
