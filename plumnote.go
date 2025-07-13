package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

var NotesFile string

type Note struct {
	Id   uint32    `json:"id"`
	Kind string    `json:"kind"`
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

func getNotesByKind(noteKind string, notes map[uint32]Note) map[uint32]Note {
	filtered := make(map[uint32]Note, 0)
	for _, note := range notes {
		if note.Kind == noteKind {
			filtered[note.Id] = note
		}
	}

	return filtered
}

func getNotesByDate(dates []string, notes map[uint32]Note) (map[uint32]Note, error) {
	filtered := make(map[uint32]Note, 0)
	format := "02/01/2006" // DD/MM/YYYY
	startDate, err := time.ParseInLocation(format, dates[0], time.Local)
	if err != nil {
		return nil, err
	}
	endDate, err := time.ParseInLocation(format, dates[1], time.Local)
	if err != nil {
		return nil, err
	}
	endDate = endDate.Add(time.Hour * 23 + time.Minute * 59 + time.Second * 59)

	for _, note := range notes {
		if startDate.Compare(note.Date.Local()) == -1 && endDate.Compare(note.Date.Local()) == 1 {
			filtered[note.Id] = note
		}
	}

	return filtered, nil
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
	var dir string
	switch runtime.GOOS {
	case "windows":
		dir = os.Getenv("%LOCALAPPDATA%")
	default:
		dir = os.Getenv("XDG_DATA_HOME")
		if dir == "" {
			panic("could not get xdg data home env, gitgud")
		}
	}

	return filepath.Join(dir, "plumnote", "notes.json")
}

func ensureStorageExists(NotesFile string) error {
	dir := filepath.Dir(NotesFile)
	return os.MkdirAll(dir, 0755)
}

func loadNotes(NotesFile string) (map[uint32]Note, error) {
	if err := ensureStorageExists(NotesFile); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(NotesFile)
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

func saveNotes(NotesFile string, notes map[uint32]Note) error {
	data, err := json.MarshalIndent(notes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(NotesFile, data, 0644)
}


func removeNote(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: plumnote r[emove] --id <id>")
	}

	var id uint32
	var err error
	for i := 0; i < len(args); i++ {
		if args[i] == "-i" || args[i] == "--id" && i+1 < len(args) {
			idconv, err := strconv.Atoi(args[i+1])
			id = uint32(idconv)
			if err != nil {
				return err
			}
		}
	}

	notes, err := loadNotes(NotesFile)
	if err != nil {
		return err
	}

	for _, note := range notes {
		if note.Id == id {
			delete(notes, note.Id)
		}
	}
	saveNotes(NotesFile, notes)

	return nil
}

func addNote(args []string) error {
	if len(args) < 3 {
		return errors.New("usage: plumnote a[dd] --kind <kind> [--tags <tags>] \"note text\"")
	}

	var noteKind string
	var tags []string
	var text string

	for i := 0; i < len(args); i++ {
		if args[i] == "-k" || args[i] == "--kind" && i+1 < len(args) {
			noteKind = args[i+1]
			i++
		} else if args[i] == "-t" || args[i] == "--tags" && i+1 < len(args) {
			tags = strings.Split(args[i+1], ",")
			i++
		} else if !strings.HasPrefix(args[i], "--") {
			text = args[i]
		}
	}

	if noteKind == "" || text == "" {
		return errors.New("you must provide --kind and the note text")
	}

	notes, err := loadNotes(NotesFile)
	if err != nil {
		return err
	}

	id := uint32(getHighestId(notes) + 1)

	notes[id] = Note{
		Id:   id,
		Kind: noteKind,
		Tags: tags,
		Text: text,
		Date: time.Now(),
	}

	return saveNotes(NotesFile, notes)
}

func filterNotes(filterMode string, filter string, notes map[uint32]Note) (map[uint32]Note, error) {
	var err error
	filteredNotes := make(map[uint32]Note, len(notes))
	switch filterMode {
	case "-i", "--id":
		var id int; id, err = strconv.Atoi(filter);
		filteredNotes[0] = notes[uint32(id)]
	case "-k", "--kind":
		filteredNotes = getNotesByKind(filter, notes)
	case "-t", "--tags":
		tags := strings.Split(filter, ",")
		filteredNotes = getNotesByTags(tags, notes)
	case "-e", "--exact-tags":
		tags := strings.Split(filter, ",")
		filteredNotes = getNotesByTagsExact(tags, notes)
	case "-d", "--date":
		dates := strings.Split(filter, ",")
		if len(dates) != 2 {
			return nil, errors.New("usage: plumnote l[ist] --date DD/MM/YYYY,DD/MM/YYYY")
		}
		filteredNotes, err = getNotesByDate(dates, notes)
	default:
		return nil, errors.New("usage: plumnote l[ist] --[id, kind, tags, exact-tags, date] <value>")
	}

	if err != nil {
		return nil, err
	}

	return filteredNotes, nil
}


func listNotes(args []string) error {
	if len(args) == 1 || len(args) > 2 {
		return errors.New("usage: plumnote l[ist] --[id, kind, tags, exact-tags, date] <value>")
	}

	notes, err := loadNotes(NotesFile)
	if err != nil {
		return err
	}

	if len(args) == 2 {
		notes, err = filterNotes(args[0], args[1], notes)
		if err != nil {
			return err
		}
	}

	if len(notes) < 1 {
		fmt.Println("no notes found.")
		return nil
	}

	format := "Monday, 2 January 2006 at 15:04:05 | "
	for _, note := range notes {
		fmt.Printf("id: %d | ", note.Id)
		fmt.Print(strings.ToLower(note.Date.Format(format)))
		fmt.Printf("kind: %s | ", note.Kind)
		if len(note.Tags) > 0 {
			fmt.Printf("tags: [%s]", strings.Join(note.Tags, ", "))
		}
		fmt.Printf("\n'%s'\n", note.Text)
		fmt.Println()
	}

	return nil
}

func updateNote(args []string) error {
	if len(args) < 3 {
		return errors.New("usage: plumnote update <id> --[tags, note, kind] <value>")
	}

	var id uint32
	var err error
	idconv, err := strconv.Atoi(args[0])
	id = uint32(idconv)
	if err != nil {
		return err
	}

	notes, err := loadNotes(NotesFile)
	if err != nil {
		return err
	}
	note := notes[id]

	updateType := args[1]
	updateValue := args[2]
	switch updateType {
	case "-t", "--tags":
		note.Tags = strings.Split(updateValue, ",")
	case "-k", "--kind":
		note.Kind = updateValue
	case "-n", "--note":
		note.Text = updateValue
	default:
		return errors.New("usage: plumnote u[pdate] <id> --[tags, note, kind] <value>")
	}

	notes[id] = note
	saveNotes(NotesFile, notes)

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: plumnote <command> [options]")
		fmt.Println("available commands: l[ist], a[dd], u[pdate], r[emove]")
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]
	NotesFile = getNotesFilePath()

	var err error
	switch command {
	case "a", "add":
		err = addNote(args)
	case "l", "list":
		err = listNotes(args)
	case "r", "remove":
		err = removeNote(args)
	case "u", "update":
		err = updateNote(args)
	default:
		fmt.Printf("unknown command: %s\n", command)
		os.Exit(1)
	}

	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
