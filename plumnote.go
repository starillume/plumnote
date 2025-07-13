package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Note struct {
	Id   uint32    `json:"id"`
	Type string    `json:"type"`
	Tags []string  `json:"tags,omitempty"`
	Text string    `json:"text"`
	Date time.Time `json:"date"`
}

func getNotesByTagsExact(tags []string, notes map[uint32]Note) map[uint32]Note {
	filtered := notes
	for _, tag := range tags {
		filtered = getNotesByTag(tag, filtered)
	}

	return filtered
}

func getNotesByTags(tags []string, notes map[uint32]Note) map[uint32]Note {
	filtered := make(map[uint32]Note, 0)
	for _, tag := range tags {
		tagNotes := getNotesByTag(tag, notes)
		maps.Copy(filtered, tagNotes)
	}

	return filtered
}

func getNotesByTag(tag string, notes map[uint32]Note) map[uint32]Note {
	filtered := make(map[uint32]Note, 0)
	for _, note := range notes {
		if slices.Contains(note.Tags, tag) {
			filtered[note.Id] = note
		}
	}

	return filtered
}

func getNotesByType(noteType string, notes map[uint32]Note) map[uint32]Note {
	filtered := make(map[uint32]Note, 0)
	for _, note := range notes {
		if note.Type == noteType {
			filtered[note.Id] = note
		}
	}

	return filtered
}

func getHighestId(notes map[uint32]Note) int32 {
	var highest int32 = -1
	for _, note := range notes {
		if int32(note.Id) > highest {
			highest = int32(note.Id)
		}
	}

	return highest
}

func getNotesFilePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		panic("could not get config directory")
	}
	return filepath.Join(dir, "plumnote", "notes.json")
}

func ensureStorageExists(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0755)
}

func loadNotes(path string) (map[uint32]Note, error) {
	if err := ensureStorageExists(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[uint32]Note{}, nil
	}
	if err != nil {
		return nil, err
	}
	var notes map[uint32]Note
	err = json.Unmarshal(data, &notes)
	return notes, err
}

func saveNotes(path string, notes map[uint32]Note) error {
	data, err := json.MarshalIndent(notes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}


func removeNote(args []string, path string) error {
	if len(args) < 2 {
		return errors.New("usage: plumnote remove --id <id>")
	}

	var id uint32
	var err error
	for i := 0; i < len(args); i++ {
		if args[i] == "--id" && i+1 < len(args) {
			idconv, err := strconv.Atoi(args[i+1])
			id = uint32(idconv)
			if err != nil {
				return err
			}
		}
	}

	notes, err := loadNotes(path)
	if err != nil {
		return err
	}

	for _, note := range notes {
		if note.Id == id {
			delete(notes, note.Id)
		}
	}
	saveNotes(path, notes)

	return nil
}

func addNote(args []string, path string) error {
	if len(args) < 3 {
		return errors.New("usage: plumnote add --type <type> [--tags <tags>] \"note text\"")
	}

	var noteType string
	var tags []string
	var text string

	for i := 0; i < len(args); i++ {
		if args[i] == "--type" && i+1 < len(args) {
			noteType = args[i+1]
			i++
		} else if args[i] == "--tags" && i+1 < len(args) {
			tags = strings.Split(args[i+1], ",")
			i++
		} else if !strings.HasPrefix(args[i], "--") {
			text = args[i]
		}
	}

	if noteType == "" || text == "" {
		return errors.New("you must provide --type and the note text")
	}

	notes, err := loadNotes(path)
	if err != nil {
		return err
	}

	id := uint32(getHighestId(notes) + 1)

	notes[id] = Note{
		Id:   id,
		Type: noteType,
		Tags: tags,
		Text: text,
		Date: time.Now(),
	}

	return saveNotes(path, notes)
}

func filterNotes(filterMode string, filter string, notes map[uint32]Note) (map[uint32]Note, error) {
	var err error
	filteredNotes := make(map[uint32]Note, len(notes))
	switch filterMode {
	case "--id":
		var id int; if id, err = strconv.Atoi(filter); err != nil { return nil, err }
		filteredNotes[0] = notes[uint32(id)]
	case "--type":
		filteredNotes = getNotesByType(filter, notes)
	case "--tags":
		tags := strings.Split(filter, ",")
		filteredNotes = getNotesByTags(tags, notes)
	case "--exact-tags":
		tags := strings.Split(filter, ",")
		filteredNotes = getNotesByTagsExact(tags, notes)
	case "--date":
	default:
		return nil, errors.New("usage: plumnote list --[id, type, tag] <value>")
	}

	return filteredNotes, nil
}


func listNotes(args []string, path string) error {
	if len(args) == 1 || len(args) > 2 {
		return errors.New("usage: plumnote list --[id, type, tags] <value>")
	}

	notes, err := loadNotes(path)
	if err != nil {
		return err
	}

	if len(args) == 2 {
		notes, err = filterNotes(args[0], args[1], notes)
		if err != nil {
			return nil
		}
	}

	if len(notes) < 1 {
		fmt.Println("no notes found.")
		return nil
	}

	format := "monday, january 2nd 2006 at 15:04:05 | "
	for _, note := range notes {
		fmt.Printf("id: %d | ", note.Id)
		fmt.Print(note.Date.Format(format))
		fmt.Printf("type: %s | ", note.Type)
		if len(note.Tags) > 0 {
			fmt.Printf("tags: [%s]", strings.Join(note.Tags, ", "))
		}
		fmt.Printf("\n'%s'\n", note.Text)
		fmt.Println()
	}

	return nil
}

func updateNote(args []string, path string) error {
	if len(os.Args) < 3 {
		return errors.New("usage: plumnote update <id> --[tags, text, type] <value>")
	}

	var id uint32
	var err error
	idconv, err := strconv.Atoi(args[0])
	id = uint32(idconv)
	if err != nil {
		return err
	}

	notes, err := loadNotes(path)
	if err != nil {
		return err
	}
	note := notes[id]

	updateType := args[1]
	updateValue := args[2]
	switch updateType {
	case "--tags":
		note.Tags = strings.Split(updateValue, ",")
	case "--type":
		note.Type = updateValue
	case "--text":
		note.Text = updateValue
	default:
		return errors.New("usage: plumnote update <id> --[tags, text, type] <value>")
	}

	notes[id] = note
	saveNotes(path, notes)

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: plumnote <command> [options]")
		fmt.Println("available commands: list, add, update, remove")
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]
	notesFile := getNotesFilePath()

	var err error
	switch command {
	case "add":
		err = addNote(args, notesFile)
	case "list":
		err = listNotes(args, notesFile)
	case "remove":
		err = removeNote(args, notesFile)
	case "update":
		err = updateNote(args, notesFile)
	default:
		fmt.Printf("unknown command: %s\n", command)
		os.Exit(1)
	}

	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
