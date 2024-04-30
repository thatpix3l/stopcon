package entrypoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/thatpix3l/stopcon/src/cmd"
	"github.com/thatpix3l/stopcon/src/ff"
	"github.com/thatpix3l/stopcon/src/format"
)

var (
	styleExample     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#00225c", Dark: "#52aeff"}).Bold(true)
	styleError       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6e1a00", Dark: "#ffab91"})
	styleDestination = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1b523d", Dark: "#78ffd6"}).Bold(true)
	styleBold        = lipgloss.NewStyle().Bold(true)
)

var root = cmd.CmdRoot{}

type Metadata struct {
	Codec        string
	CreationTime *time.Time
}

func (m Metadata) CreationTimeString() string {
	return m.CreationTime.Format("2006-01-02 15_04_05")
}

type Video struct {
	Id string
	Metadata
}

type Fragment struct {
	Video
	Index       int    // Video index for a complete video.
	Extension   string // File name extension.
	CurrentName string // File name as-is.
	NewName     string // File name for renaming purposes.
}

// Absolute path to [Fragment]'s current location.
func (f Fragment) InputPath() string {
	return filepath.Join(root.Rename.InputDirPath, f.CurrentName)
}

// Absolute path to [Fragment]'s new location, for renaming purposes.
func (f Fragment) NewPath() string {
	return filepath.Join(root.Rename.InputDirPath, f.NewName)
}

func cmdAdapter[Slice any, Output any](callback func(Slice, ...Slice) Output, c []Slice) Output {

	var output Output

	if len(c) < 1 {
		return output
	}

	first := c[0]

	startIndex := 0
	if len(c) >= 2 {
		startIndex = 1
	}

	rest := c[startIndex:]

	return callback(first, rest...)
}

func ffprobeCmd(path string) []string {
	return []string{
		"ffprobe", path,
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-select_streams", "v:0",
		"-hide_banner",
		"-loglevel", "fatal",
	}
}

// Parse and store embedded video [Fragment] metadata.
func (f *Fragment) parseMetadata() error {

	jsonBuf, err := cmdAdapter(exec.Command, ffprobeCmd(f.InputPath())).Output()
	if err != nil {
		return err
	}

	data := ff.ProbeData{}
	if err := json.Unmarshal(jsonBuf, &data); err != nil {
		return err
	}

	// Extract what we care from structure
	codec := data.Streams[0].CodecName

	creationTimeV, ok := data.Format.Tags["creation_time"]
	if !ok {
		return errors.New("tag \"creation_time\"not embedded in video")
	}

	creationTimeStr, ok := creationTimeV.(string)
	if !ok {
		return errors.New("tag \"creation_time\" is not a string")
	}

	// Parse timestamp into Go's time.Time, cache for later use
	creationTime, err := time.Parse("2006-01-02T15:04:05.9Z", creationTimeStr)
	if err != nil {
		return err
	}

	// Store into video [Fragment]
	f.Metadata.Codec = codec
	f.Metadata.CreationTime = &creationTime

	return nil
}

// Parser for GoPro-named partial recordings.
func (f *Fragment) parseRaw() error {

	// Get matches based off of [Fragment]'s name.
	matches := format.Raw.Regex.FindStringSubmatch(f.CurrentName)
	if len(matches) < len(format.Raw.Tokens.Slice) {
		return errors.New("cannot parse as raw name")
	}

	index, err := strconv.Atoi(matches[2])
	if err != nil {
		return errors.New("cannot parse index as an integer")
	}

	f.Id = matches[3]
	f.Index = index
	f.Extension = matches[4]

	return nil
}

// Parser for preferred-name partial recordings.
func (f *Fragment) parseRenamed() error {

	// Get matches based off of [Fragment]'s name.
	matches := format.Renamed.Regex.FindStringSubmatch(f.CurrentName)
	if len(matches) < len(format.Renamed.Tokens.Slice) {
		return errors.New("cannot parse as pretty name")
	}

	index, err := strconv.Atoi(matches[format.Renamed.Tokens.Map["index"].Index+1])
	if err != nil {
		return err
	}

	f.Id = matches[format.Renamed.Tokens.Map["id"].Index+1]
	f.Index = index
	f.Extension = matches[format.Renamed.Tokens.Map["extension"].Index+1]

	return nil
}

// Parser for preferred-name merged recordings.
func (f *Fragment) parseMerged() error {
	matches := format.Merged.Regex.FindStringSubmatch(f.CurrentName)
	if len(matches) < len(format.Merged.Tokens.Slice) {
		return errors.New("cannot parse as merged name")
	}

	f.Id = matches[2]
	f.Extension = matches[3]

	return nil

}

func GetFunctionName(i interface{}) (string, error) {

	var out string

	v := reflect.ValueOf(i)

	if v.Type().Kind() != reflect.Func {
		return out, errors.New("")
	}

	out = runtime.FuncForPC(v.Pointer()).Name()

	return out, nil
}

// Parse fragment by its name and embedded metadata.
func (f *Fragment) Parse() error {

	nameParsers := []func() error{f.parseRenamed, f.parseRaw, f.parseMerged}

	for _, nameParser := range nameParsers {
		if err := nameParser(); err == nil {

			if err := f.parseMetadata(); err != nil {
				return err
			}

			f.NewName = fmt.Sprintf(format.Renamed.Layout, f.CreationTimeString(), f.Id, f.Index, f.Extension)

			return nil

		}
	}

	return errors.New("name not parseable")

}

// Merged [Video], composed of one or more [Fragment]s
type Merged struct {
	Video
	Fragments []Fragment // Individual video fragments that, when merged together, create a whole video.
	Expected  int        // Total expected fragments for merged video.
	name      string     // Cached pretty name
}

func (m *Merged) Name() string {

	if m.name == "" {
		m.name = fmt.Sprintf(format.Merged.Layout, m.CreationTimeString(), m.Id)
	}

	return m.name
}

type Videos map[string]*Merged

var videosMutex = sync.RWMutex{}

// Add entry as a new video [Fragment].
func (v Videos) Add(name string) error {

	f := Fragment{CurrentName: name}

	if err := f.Parse(); err != nil {
		return err
	}

	videosMutex.Lock()
	defer videosMutex.Unlock()

	// Initialize video if never created for current f's ID
	if _, ok := v[f.Id]; !ok {
		v[f.Id] = &Merged{
			Video:     f.Video,
			Fragments: []Fragment{},
		}
	}

	// Address of current merged
	merged := v[f.Id]

	// If video already contains date, assign it to [Fragment]; otherwise, parse and set both.
	if merged.CreationTime != nil {
		f.CreationTime = merged.CreationTime

	} else {
		merged.CreationTime = f.CreationTime
	}

	// Error if codec differs from each other
	if merged.Codec != f.Codec {
		return errors.New("merged and fragment video with ID's have differing video codecs")
	}

	// Store current [Fragment] into video
	merged.Fragments = append(merged.Fragments, f)

	// Update expected count of [Fragment]s if necessary
	if f.Index > merged.Expected {
		merged.Expected = f.Index
	}

	return nil

}

// Print what will be renamed.
func renameInfo(old string, new string) error {

	fmt.Printf("%4s\n%s\n%4s\n%s\n", styleBold.Render("From"), old, styleBold.Render("To"), styleDestination.Render(new))

	return nil
}

// Rename old file into new file.
func renameCommit(old string, new string) error {

	if err := os.Rename(old, new); err != nil {
		return err
	}

	return nil
}

func renameActionBuilder(actionList ...func(old string, new string) error) func(old string, new string) error {
	return func(old, new string) error {

		if old == new {

			log.Infof("Already renamed: %s", old)

			return nil
		}

		for _, action := range actionList {
			if err := action(old, new); err != nil {
				return err
			}
		}

		return nil
	}
}

var entries = Videos{}

func rename() error {

	videoEntries, err := os.ReadDir(root.Rename.InputDirPath)
	if err != nil {
		return err
	}

	renameMessage := "Renaming (Dry Run)"
	if root.Rename.Commit {
		renameMessage = "Renaming"
	}

	entriesWG := sync.WaitGroup{}

	// For each entry in input directory...
	for _, entry := range videoEntries {

		entriesWG.Add(1)

		// Parse and add entry to list of video entries, store error if any.
		go func(e fs.DirEntry) {
			defer entriesWG.Done()
			if err := entries.Add(e.Name()); err != nil {
				log.Warnf("entry %s cannot be added: %v", styleExample.Render(e.Name()), styleError.Render(err.Error()))
			}
		}(entry)

	}

	entriesWG.Wait()

	// Error if no entries to process
	if len(entries) == 0 {
		return fmt.Errorf("directory does not contain GoPro-named videos")
	}

	// Print renaming message
	fmt.Printf("%s\n\n", renameMessage)

	// Set default renaming action: only print, don't actually rename.
	renameAction := renameInfo

	// Set renaming function to also rename if specified by user
	if root.Rename.Commit {
		renameAction = renameActionBuilder(renameInfo, renameCommit)
	}

	// Run rename action on each video [Fragment]
	totalFragments := 0
	for _, v := range entries {
		for _, c := range v.Fragments {
			old := c.InputPath()
			new := c.NewPath()

			if totalFragments > 0 {
				fmt.Println()
			}

			totalFragments++

			if err := renameAction(old, new); err != nil {
				log.Warn("%v", err)
				continue
			}

		}
	}

	return nil
}

func Main() {

	log.SetLevel(log.DebugLevel)

	// Parse options
	arg.MustParse(&root)

	// Post process of command stuff
	if err := root.PostProcess(); err != nil {
		log.Errorf("%v", err)
		return
	}

	// Rename all video files in directory matching GoPro video file naming convention
	if err := rename(); err != nil {
		log.Errorf("%v", err)
		return
	}

}
