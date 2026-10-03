package report

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// Internal describes the beets database itself.
type Internal struct {
	DBBytes         int64       `json:"dbBytes"`
	Tables          []Count     `json:"tables"` // row counts
	Attributes      []Attribute `json:"attributes"`
	Fields          []Check     `json:"fields"`          // optional track fields that plugins fill
	DuplicateAlbums int         `json:"duplicateAlbums"` // groups of albums with the same artist and name
}

// Attribute is a flexible field, set by plugins or by this tool.
type Attribute struct {
	Entity string `json:"entity"`
	Key    string `json:"key"`
	Count  int    `json:"count"`
}

var internalTables = []string{"items", "albums", "item_attributes", "album_attributes"}

var optionalFields = []struct{ label, column string }{
	{"MusicBrainz track ID", "mb_trackid"},
	{"MusicBrainz album ID", "mb_albumid"},
	{"MusicBrainz artist ID", "mb_artistid"},
	{"AcoustID", "acoustid_id"},
	{"ISRC", "isrc"},
	{"ReplayGain (track)", "rg_track_gain"},
	{"ReplayGain (album)", "rg_album_gain"},
	{"Lyrics", "lyrics"},
	{"BPM", "bpm"},
	{"Initial key", "initial_key"},
	{"Label", "label"},
	{"Composer", "composer"},
	{"Media", "media"},
	{"Comments", "comments"},
}

func collectInternal(db *sql.DB, dbPath string) (Internal, error) {
	var in Internal
	fi, err := os.Stat(dbPath)
	if err != nil {
		return in, fmt.Errorf("stat beets db: %w", err)
	}
	in.DBBytes = fi.Size()

	for _, t := range internalTables {
		var n int
		if err = db.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&n); err != nil {
			return in, fmt.Errorf("count %s: %w", t, err)
		}
		in.Tables = append(in.Tables, Count{t, n})
	}

	for _, entity := range []string{"item", "album"} {
		rows, err := db.Query("SELECT x.key, COUNT(*) FROM " + entity + "_attributes x GROUP BY x.key ORDER BY COUNT(*) DESC, x.key")
		if err != nil {
			return in, fmt.Errorf("query %s attributes: %w", entity, err)
		}
		for rows.Next() {
			a := Attribute{Entity: entity}
			if err = rows.Scan(&a.Key, &a.Count); err != nil {
				rows.Close()
				return in, fmt.Errorf("scan attribute: %w", err)
			}
			in.Attributes = append(in.Attributes, a)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return in, err
		}
		rows.Close()
	}

	exprs := make([]string, len(optionalFields))
	for i, f := range optionalFields {
		exprs[i] = fmt.Sprintf("COALESCE(SUM(i.%s IS NOT NULL AND i.%s != '' AND i.%s != 0), 0)", f.column, f.column, f.column)
	}
	counts := make([]int, len(optionalFields)+1)
	dest := make([]any, len(counts))
	for i := range counts {
		dest[i] = &counts[i]
	}
	if err = db.QueryRow("SELECT COUNT(*), " + strings.Join(exprs, ", ") + " FROM items i").Scan(dest...); err != nil {
		return in, fmt.Errorf("query field coverage: %w", err)
	}
	for i, f := range optionalFields {
		in.Fields = append(in.Fields, Check{Label: f.label, Missing: counts[0] - counts[i+1], Total: counts[0]})
	}
	return in, nil
}
