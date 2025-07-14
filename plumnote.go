package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

var NotesFile string
var SettingsFile string
var SettingsTemplate Settings = map[string]string{
	"author":     "",
	"syncserver": "",
}

type Note struct {
	Id     int64     `json:"id"`
	Kind   string    `json:"kind"`
	Tags   []string  `json:"tags,omitempty"`
	Text   string    `json:"text"`
	Date   time.Time `json:"date"`
	Synced bool      `json:"synced"`
	Author string    `json:"author"`
}

type Settings map[string]string
type Notes map[int64]Note

func getNotesByTagsExact(tags []string, notes Notes) Notes {
	filtered := notes
	for _, tag := range tags {
		filtered = getNotesByTag(tag, filtered)
	}

	return filtered
}

func getNotesByTags(tags []string, notes Notes) Notes {
	filtered := make(Notes, 0)
	for _, tag := range tags {
		tagNotes := getNotesByTag(tag, notes)
		maps.Copy(filtered, tagNotes)
	}

	return filtered
}

func getNotesByTag(tag string, notes Notes) Notes {
	filtered := make(Notes, 0)
	for _, note := range notes {
		if slices.Contains(note.Tags, tag) {
			filtered[note.Id] = note
		}
	}

	return filtered
}

func getNotesByKind(noteKind string, notes Notes) Notes {
	filtered := make(Notes, 0)
	for _, note := range notes {
		if note.Kind == noteKind {
			filtered[note.Id] = note
		}
	}

	return filtered
}

func getNotesByDate(dates []string, notes Notes) (Notes, error) {
	filtered := make(Notes, 0)
	format := "02/01/2006" // DD/MM/YYYY
	startDate, err := time.ParseInLocation(format, dates[0], time.Local)
	if err != nil {
		return nil, err
	}
	endDate, err := time.ParseInLocation(format, dates[1], time.Local)
	if err != nil {
		return nil, err
	}
	endDate = endDate.Add(time.Hour*23 + time.Minute*59 + time.Second*59)

	for _, note := range notes {
		if startDate.Compare(note.Date.Local()) == -1 && endDate.Compare(note.Date.Local()) == 1 {
			filtered[note.Id] = note
		}
	}

	return filtered, nil
}

func getNotesByAuthor(author string, notes Notes) Notes {
	filtered := make(Notes, 0)
	for _, note := range notes {
		if note.Author == author {
			filtered[note.Id] = note
		}
	}

	return filtered
}

func getNotesFilePath() string {
	var dir string
	switch runtime.GOOS {
	case "windows":
		dir, _ = os.UserConfigDir()
	default:
		dir = os.Getenv("XDG_DATA_HOME")
		if dir == "" {
			panic("could not get xdg data home env, gitgud")
		}
	}

	return filepath.Join(dir, "plumnote", "notes.json")
}

func getSettingsFilePath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "plumnote", "settings.json")
}

func ensureStorageExists(NotesFile string) error {
	dir := filepath.Dir(NotesFile)
	return os.MkdirAll(dir, 0755)
}

func load[T any](filepath string, alocatedT T) error {
	if err := ensureStorageExists(filepath); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, &alocatedT)
	return err
}

func save[T any](filepath string, t T) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, data, 0644)
}

func removeNote(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: plumnote r[emove] --id <id>")
	}

	var id int64
	var err error
	for i := 0; i < len(args); i++ {
		if args[i] == "-i" || args[i] == "--id" && i+1 < len(args) {
			idconv, err := strconv.Atoi(args[i+1])
			id = int64(idconv)
			if err != nil {
				return err
			}
		}
	}
	notes := make(Notes, 0)
	err = load(NotesFile, notes)
	if err != nil {
		return err
	}

	for _, note := range notes {
		if note.Id == id {
			delete(notes, note.Id)
		}
	}
	save(NotesFile, notes)

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

	notes := make(Notes, 0)
	err := load(NotesFile, notes)
	if err != nil {
		return err
	}
	settings := make(Settings, 0)
	err = load(SettingsFile, settings)
	if err != nil {
		return err
	}

	id := time.Now().Unix()
	notes[id] = Note{
		Id:     id,
		Kind:   noteKind,
		Tags:   tags,
		Text:   text,
		Date:   time.Now(),
		Author: settings["author"],
	}

	return save(NotesFile, notes)
}

func filterNotes(filterMode string, filter string, notes Notes) (Notes, error) {
	var err error
	filteredNotes := make(Notes, len(notes))
	switch filterMode {
	case "-a", "--author":
		filteredNotes = getNotesByAuthor(filter, notes)
	case "-i", "--id":
		var id int
		id, err = strconv.Atoi(filter)
		filteredNotes[0] = notes[int64(id)]
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
		return nil, errors.New("usage: plumnote l[ist] --[id, kind, tags, exact-tags, date, author] <value>")
	}

	if err != nil {
		return nil, err
	}

	return filteredNotes, nil
}

func listNotes(args []string) error {
	if len(args) == 1 {
		return errors.New("usage: plumnote l[ist] --[id, kind, tags, exact-tags, date, author] <value>")
	}
	notes := make(Notes, 0)
	err := load(NotesFile, notes)
	if err != nil {
		return err
	}

	if len(args) >= 2 {
		for i := 0; i < len(args); i = i + 2 {
			if i+1 >= len(args) {
				return errors.New("usage: plumnote l[ist] --[id, kind, tags, exact-tags, date, author] <value>")
			}
			notes, err = filterNotes(args[i], args[i+1], notes)
			if err != nil {
				return err
			}
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
		if note.Author != "" {
			fmt.Printf(" | by: %s", note.Author)
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

	var id int64
	var err error
	idconv, err := strconv.Atoi(args[0])
	id = int64(idconv)
	if err != nil {
		return err
	}

	notes := make(Notes, 0)
	err = load(NotesFile, notes)
	if err != nil {
		return err
	}
	note := notes[id]

	settings := make(Settings, 0)
	err = load(SettingsFile, settings)
	if err != nil {
		return err
	}
	author := settings["author"]
	if note.Author != author {
		return errors.New("can't update other's notes!")
	}

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

	note.Synced = false
	note.Date = time.Now()
	notes[id] = note
	save(NotesFile, notes)

	return nil
}

func setSettingsValue(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: plumnote s[ettings] <key> <value>")
	}

	key := args[0]
	value := args[1]
	settings := make(Settings, 0)
	err := load(SettingsFile, settings)
	if err != nil {
		return err
	}

	if _, ok := SettingsTemplate[key]; !ok {
		return errors.New("usage: plumnote s[ettings] <key> <value>")
	}

	settings[key] = value
	save(SettingsFile, settings)
	return nil
}

type NoteToSync struct {
	Id     int64     `json:"id"`
	Kind   string    `json:"kind"`
	Tags   []string  `json:"tags,omitempty"`
	Text   string    `json:"text"`
	Date   time.Time `json:"date"`
	Author string    `json:"author"`
}

func notesReceiveToSync(nts []NoteToSync) error {
	notes := make(Notes, 0)
	err := load(NotesFile, notes)
	if err != nil {
		return err
	}

	for _, ns := range nts {
		note := Note{
			Id:     ns.Id,
			Kind:   ns.Kind,
			Tags:   ns.Tags,
			Text:   ns.Text,
			Synced: true,
			Date:   ns.Date,
			Author: ns.Author,
		}

		notes[ns.Id] = note
	}

	save(NotesFile, notes)

	return nil
}

func notesToSync() ([]byte, error) {
	notes := make(Notes, 0)
	err := load(NotesFile, notes)
	if err != nil {
		return nil, err
	}

	toSync := make([]NoteToSync, 0)
	for _, n := range notes {
		if !n.Synced {
			ns := NoteToSync{
				Id:     n.Id,
				Kind:   n.Kind,
				Tags:   n.Tags,
				Text:   n.Text,
				Date:   n.Date,
				Author: n.Author,
			}
			toSync = append(toSync, ns)
			n.Synced = true
			notes[n.Id] = n
		}
	}

	toSyncJson, err := json.MarshalIndent(toSync, "", "  ")
	if err != nil {
		return nil, err
	}

	save(NotesFile, notes)

	return toSyncJson, nil
}

func syncHandlerFunc(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		fmt.Fprintf(w, "method not allowed")
		return
	}

	if r.ContentLength == -1 {
		w.WriteHeader(400)
		fmt.Fprintf(w, "ta errado ai fio")
		return
	}

	if r.ContentLength > 1000000000 { // 1gb
		w.WriteHeader(400)
		fmt.Fprintf(w, "pode não")
		return
	}

	body := make([]byte, r.ContentLength)

	r.Body.Read(body)

	var notes []NoteToSync
	if err := json.Unmarshal(body, &notes); err != nil {
		w.WriteHeader(400)
		fmt.Fprintf(w, err.Error())
		return
	}

	if err := notesReceiveToSync(notes); err != nil {
		w.WriteHeader(400)
		fmt.Fprintf(w, err.Error())
		return
	}

	notesToSend, err := notesToSync()
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, err.Error())
		return
	}

	fmt.Fprintf(w, string(notesToSend))
	fmt.Println("notes synced!")
}

func startDaemon(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: plumnote d[sync] [port]")
	}

	port := 8080
	if len(args) == 1 {
		var err error
		port, err = strconv.Atoi(args[0])
		if err != nil {
			return err
		}
	}

	http.HandleFunc("/sync", syncHandlerFunc)
	fmt.Printf("listening in port %d...\n", port)
	return http.ListenAndServe(":"+fmt.Sprint(port), nil)
}

func sendRequest(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: plumnote p[sync] [ip:port]")
	}
	var ip, port string
	var split []string
	if len(args) == 1 {
		split = strings.Split(args[0], ":")
	} else {
		settings := make(Settings, 0)
		err := load(SettingsFile, settings)
		if err != nil {
			return err
		}
		var ok bool
		var syncserver string
		if syncserver, ok = settings["syncserver"]; !ok {
			return errors.New("settings.syncserver value not found. please set it with plumnote s syncserver [ip:port].")
		}
		split = strings.Split(syncserver, ":")
	}

	ip, port = split[0], split[1]
	url := fmt.Sprintf("http://%s:%s/sync", ip, port)
	notesToSend, err := notesToSync()
	if err != nil {
		return err
	}

	res, err := http.Post(url, "application/json", bytes.NewBuffer(notesToSend))
	if err != nil {
		return err
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	notesReceived := make([]NoteToSync, 0)
	if err := json.Unmarshal(body, &notesReceived); err != nil {
		return err
	}

	err = notesReceiveToSync(notesReceived)
	if err != nil {
		return err
	}
	
	fmt.Println("notes synced!")
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: plumnote <command> [options]")
		fmt.Println("available commands: l[ist], a[dd], u[pdate], r[emove], s[ettings], d[sync], p[sync]")
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]
	NotesFile = getNotesFilePath()
	SettingsFile = getSettingsFilePath()

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
	case "s", "settings":
		err = setSettingsValue(args)
	case "d", "dsync":
		err = startDaemon(args)
	case "p", "psync":
		err = sendRequest(args)
	default:
		fmt.Printf("unknown command: %s\n", command)
		os.Exit(1)
	}

	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
