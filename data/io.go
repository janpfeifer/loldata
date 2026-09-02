package data

import (
	"bufio"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func init() {
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})
	gob.Register(time.Time{})
	gob.Register(Side(0))
	gob.Register(Position(0))
	gob.Register(DataCompleteness(0))
	gob.Register(DragonType(0))
}

// datasetStorageDTO is the internal struct serialized to/from Gob storage.
type datasetStorageDTO struct {
	Matches   []*MatchV5    `json:"matches"`
	Summoners []*SummonerV4 `json:"summoners"`
}

// isGzip returns true if the filePath ends with ".gz" (ignoring temporary suffixes like "~" or ".tmp"), case-insensitively.
func isGzip(filePath string) bool {
	clean := strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(filePath), "~"), ".tmp")
	return strings.HasSuffix(clean, ".gz")
}

// WriteJSON serializes the dataset's matches and summoners as formatted JSON to the given writer.
func (d *Dataset) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

// ReadJSON deserializes matches and summoners from a JSON reader into the dataset,
// restoring indexing and bidirectional references.
func (d *Dataset) ReadJSON(r io.Reader) error {
	var loaded Dataset
	dec := json.NewDecoder(r)
	if err := dec.Decode(&loaded); err != nil {
		return err
	}
	d.mergeLoaded(&loaded)
	return nil
}

// LoadJSON is an alias for ReadJSON.
func (d *Dataset) LoadJSON(r io.Reader) error {
	return d.ReadJSON(r)
}

// WriteGob serializes the dataset's matches and summoners using gob binary encoding to the given writer.
func (d *Dataset) WriteGob(w io.Writer) error {
	// Temporarily clear participant.Summoner to prevent redundant object duplication in gob
	for _, m := range d.Matches {
		if m != nil {
			for _, p := range m.Info.Participants {
				if p != nil {
					p.Summoner = nil
				}
			}
		}
	}
	defer func() {
		for _, m := range d.Matches {
			if m != nil {
				for _, p := range m.Info.Participants {
					if p != nil && p.PUUID != "" {
						p.Summoner = d.PUUIDToSummoner[p.PUUID]
					}
				}
			}
		}
	}()

	dto := datasetStorageDTO{
		Matches:   d.Matches,
		Summoners: d.Summoners,
	}
	enc := gob.NewEncoder(w)
	return enc.Encode(&dto)
}

// ReadGob deserializes matches and summoners from a gob binary reader into the dataset,
// restoring indexing and bidirectional references.
func (d *Dataset) ReadGob(r io.Reader) error {
	var loaded datasetStorageDTO
	dec := gob.NewDecoder(r)
	if err := dec.Decode(&loaded); err != nil {
		return err
	}
	d.mergeLoaded(&Dataset{
		Matches:   loaded.Matches,
		Summoners: loaded.Summoners,
	})
	return nil
}

// LoadGob is an alias for ReadGob.
func (d *Dataset) LoadGob(r io.Reader) error {
	return d.ReadGob(r)
}

// SaveToJSON serializes the dataset's matches and summoners to a JSON file.
// If the filePath ends with ".gz", it automatically compresses the output using gzip.
func (d *Dataset) SaveToJSON(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create JSON file %q: %w", filePath, err)
	}
	defer file.Close()

	bw := bufio.NewWriter(file)
	if isGzip(filePath) {
		gw := gzip.NewWriter(bw)
		if err := d.WriteJSON(gw); err != nil {
			return fmt.Errorf("failed to encode dataset to JSON %q: %w", filePath, err)
		}
		if err := gw.Close(); err != nil {
			return fmt.Errorf("failed to close gzip writer for %q: %w", filePath, err)
		}
	} else {
		if err := d.WriteJSON(bw); err != nil {
			return fmt.Errorf("failed to encode dataset to JSON %q: %w", filePath, err)
		}
	}

	if err := bw.Flush(); err != nil {
		return fmt.Errorf("failed to flush data to JSON file %q: %w", filePath, err)
	}
	d.Saved = true
	return nil
}

// LoadFromJSON deserializes matches and summoners from a JSON file into the dataset,
// restoring indexing and bidirectional references.
// If the filePath ends with ".gz", it automatically decompresses the input using gzip.
func (d *Dataset) LoadFromJSON(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open JSON file %q: %w", filePath, err)
	}
	defer file.Close()

	br := bufio.NewReader(file)
	var r io.Reader = br
	if isGzip(filePath) {
		gr, err := gzip.NewReader(br)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader for %q: %w", filePath, err)
		}
		defer gr.Close()
		r = gr
	}

	if err := d.ReadJSON(r); err != nil {
		return fmt.Errorf("failed to decode dataset from JSON %q: %w", filePath, err)
	}
	d.Saved = true
	return nil
}

// SaveToGob serializes the dataset's matches and summoners to a gob binary file.
// If the filePath ends with ".gz", it automatically compresses the output using gzip.
func (d *Dataset) SaveToGob(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create Gob file %q: %w", filePath, err)
	}
	defer file.Close()

	bw := bufio.NewWriter(file)
	if isGzip(filePath) {
		gw := gzip.NewWriter(bw)
		if err := d.WriteGob(gw); err != nil {
			return fmt.Errorf("failed to encode dataset to Gob %q: %w", filePath, err)
		}
		if err := gw.Close(); err != nil {
			return fmt.Errorf("failed to close gzip writer for %q: %w", filePath, err)
		}
	} else {
		if err := d.WriteGob(bw); err != nil {
			return fmt.Errorf("failed to encode dataset to Gob %q: %w", filePath, err)
		}
	}

	if err := bw.Flush(); err != nil {
		return fmt.Errorf("failed to flush data to Gob file %q: %w", filePath, err)
	}
	d.Saved = true
	return nil
}

// LoadFromGob deserializes matches and summoners from a gob binary file into the dataset,
// restoring indexing and bidirectional references.
// If the filePath ends with ".gz", it automatically decompresses the input using gzip.
func (d *Dataset) LoadFromGob(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open Gob file %q: %w", filePath, err)
	}
	defer file.Close()

	br := bufio.NewReader(file)
	var r io.Reader = br
	if isGzip(filePath) {
		gr, err := gzip.NewReader(br)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader for %q: %w", filePath, err)
		}
		defer gr.Close()
		r = gr
	}

	if err := d.ReadGob(r); err != nil {
		return fmt.Errorf("failed to decode dataset from Gob %q: %w", filePath, err)
	}
	d.Saved = true
	return nil
}

// mergeLoaded populates the dataset from loaded summoners and matches, establishing indexes and links.
func (d *Dataset) mergeLoaded(loaded *Dataset) {
	if d.PUUIDToSummoner == nil {
		d.PUUIDToSummoner = make(map[string]*SummonerV4)
	}
	if d.MatchIDToMatch == nil {
		d.MatchIDToMatch = make(map[string]*MatchV5)
	}

	// Register all summoners first
	for _, s := range loaded.Summoners {
		if s == nil || s.PUUID == "" {
			continue
		}
		if s.Matches == nil {
			s.Matches = make([]*MatchV5, 0)
		}
		if existing, exists := d.PUUIDToSummoner[s.PUUID]; exists {
			if existing.Name == "" && s.Name != "" {
				existing.Name = s.Name
			}
			if existing.AccountID == "" && s.AccountID != "" {
				existing.AccountID = s.AccountID
			}
			if existing.ID == "" && s.ID != "" {
				existing.ID = s.ID
			}
			if existing.SummonerLevel == 0 && s.SummonerLevel != 0 {
				existing.SummonerLevel = s.SummonerLevel
			}
			if existing.ProfileIconID == 0 && s.ProfileIconID != 0 {
				existing.ProfileIconID = s.ProfileIconID
			}
			if existing.RevisionDate == 0 && s.RevisionDate != 0 {
				existing.RevisionDate = s.RevisionDate
			}
			if !existing.Crawled && s.Crawled {
				existing.Crawled = s.Crawled
			}
		} else {
			d.Summoners = append(d.Summoners, s)
			d.PUUIDToSummoner[s.PUUID] = s
		}
	}

	// Add matches and establish links
	for _, m := range loaded.Matches {
		if m == nil {
			continue
		}
		d.AddMatch(m)
	}

	d.Saved = true
}
