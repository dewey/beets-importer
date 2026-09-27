package cmd

import (
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
)

func TestBuildTasksGroupsByAlbum(t *testing.T) {
	albums := map[int]beets.Album{
		1: {ID: 1, AlbumArtist: "amon tobin", Album: "foley room", Path: "/lib/amon tobin/foley room"},
		2: {ID: 2, AlbumArtist: "Burial", Album: "Untrue", Path: "/lib/Burial/Untrue"},
		3: {ID: 3, AlbumArtist: "Kode9", Album: "Nothing", Path: "/lib/Kode9/Nothing"},
	}
	issues := []doctor.Issue{
		{Path: "/lib/amon tobin/foley room/01.mp3", AlbumID: 1, Description: "01.mp3: lowercase"},
		{Path: "/lib/amon tobin/foley room/02.mp3", AlbumID: 1, Description: "02.mp3: lowercase"},
		{Path: "/lib/Burial/Untrue/01.mp3", AlbumID: 2, Description: "01.mp3: lowercase"},
		{Path: "/lib/Kode9/Nothing/01.mp3", AlbumID: 3, Description: "01.mp3: lowercase"},
		{Path: "/lib/Non-Album/single.mp3", Description: "single.mp3: lowercase"},
	}
	retagged := map[int]bool{3: true}

	tasks := buildTasks(fix{label: "retag", byAlbum: true, retag: true}, issues, albums, retagged)
	if len(tasks) != 2 {
		t.Fatalf("tasks = %+v, want albums 1 and 2", tasks)
	}
	if tasks[0].albumID != 1 || tasks[0].path != "/lib/amon tobin/foley room" || tasks[0].title != "amon tobin — foley room: 2 tracks" {
		t.Errorf("tasks[0] = %+v", tasks[0])
	}
	if tasks[1].albumID != 2 || tasks[1].title != "01.mp3: lowercase" {
		t.Errorf("tasks[1] = %+v", tasks[1])
	}

	folders := buildTasks(fix{label: "remove folder"}, []doctor.Issue{{Path: "/lib/a"}, {Path: "/lib/b"}}, albums, retagged)
	if len(folders) != 2 || folders[1].path != "/lib/b" {
		t.Errorf("folder tasks = %+v", folders)
	}
}
