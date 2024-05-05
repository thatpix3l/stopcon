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
	"strings"
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
	if m.CreationTime == nil {
		return ""
	}

	return m.CreationTime.Format("2006-01-02 15_04_05")
}

type Video struct {
	Id string
	Metadata
}

type VideoFragment struct {
	Video
	Index       int    // Video index for a complete video.
	Extension   string // File name extension.
	CurrentName string // File name as-is.
	NewName     string // File name for renaming purposes.
}

// Absolute path to [VideoFragment]'s current location.
func (f VideoFragment) InputPath() string {
	return filepath.Join(root.InputDirPath, f.CurrentName)
}

// Absolute path to [VideoFragment]'s new location, for renaming purposes.
func (f VideoFragment) NewPath() string {
	return filepath.Join(root.InputDirPath, f.NewName)
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

func ffmpegCmd(dest string) []string {
	return []string{
		"ffmpeg",
		"-protocol_whitelist", "file,pipe",
		"-f", "concat",
		"-safe", "0",
		"-i", "pipe:",
		"-codec", "copy",
		"-map_metadata", "0",
		dest,
	}
}

// Merge separated video fragments into a single video file.
func (m VideoWhole) merge() error {

	sources := strings.Builder{}

	for _, f := range m.Fragments {
		if _, err := sources.WriteString(fmt.Sprintf("file '%s'\n", f.InputPath())); err != nil {
			return err
		}
	}

	cmd := cmdAdapter(exec.Command, ffmpegCmd(m.OutputPath()))
	cmd.Stdin = strings.NewReader(sources.String())

	if _, err := cmd.Output(); err != nil {
		return err
	}

	return nil
}

// Parse and store embedded video [VideoFragment] metadata.
func (f *VideoFragment) parseMetadata() error {

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
func (f *VideoFragment) parseRaw() error {

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
func (f *VideoFragment) parseRenamed() error {

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
func (f *VideoFragment) parseMerged() error {
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
func (f *VideoFragment) Parse() error {

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

// VideoWhole [Video], composed of one or more [VideoFragment]s
type VideoWhole struct {
	Video
	Fragments []VideoFragment // Individual video fragments that, when merged together, create a whole video.
	Expected  int             // Total expected fragments for merged video.
	Name      string          // Cached name for video merging purposes.
}

// Absolute path to output when merging [VideoWhole].
func (vw VideoWhole) OutputPath() string {
	return filepath.Join(root.Merge.OutputDirPath, vw.Name)
}

type VideoList map[string]*VideoWhole

var videosMutex = sync.RWMutex{}

// Add entry as a new video [VideoFragment].
func (v VideoList) Add(name string) error {

	f := VideoFragment{CurrentName: name}

	if err := f.Parse(); err != nil {
		return err
	}

	videosMutex.Lock()
	defer videosMutex.Unlock()

	// Initialize video if never created for current [Fragment]'s ID
	if _, ok := v[f.Id]; !ok {
		v[f.Id] = &VideoWhole{
			Video:     f.Video,
			Fragments: []VideoFragment{},
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

	// Store current [Fragment] into video
	merged.Fragments = append(merged.Fragments, f)

	// Update expected count of [Fragment]s if necessary
	if f.Index > merged.Expected {
		merged.Expected = f.Index
	}

	if merged.Name == "" {
		merged.Name = fmt.Sprintf(format.Merged.Layout, merged.CreationTimeString(), merged.Id, "mkv")
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

var videoList = VideoList{}

func (e VideoList) Parse() error {

	dirEntries, err := os.ReadDir(root.InputDirPath)
	if err != nil {
		return err
	}

	addWG := sync.WaitGroup{}

	// For each entry in input directory...
	for _, entry := range dirEntries {

		addWG.Add(1)

		// Parse and add entry to list of video entries, store error if any.
		go func(e fs.DirEntry) {
			defer addWG.Done()
			if err := videoList.Add(e.Name()); err != nil {
				log.Warnf("entry %s cannot be added: %v", styleExample.Render(e.Name()), styleError.Render(err.Error()))
			}
		}(entry)

	}

	addWG.Wait()

	// Error if no videos to process
	if len(videoList) == 0 {
		return fmt.Errorf("directory does not contain GoPro-named videos")
	}

	return nil
}

func rename() error {

	if err := videoList.Parse(); err != nil {
		return err
	}

	renameMessage := "Renaming (Dry Run)"
	if root.Rename.Commit {
		renameMessage = "Renaming"
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
	for _, vm := range videoList {
		for _, vf := range vm.Fragments {
			old := vf.InputPath()
			new := vf.NewPath()

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

func merge() error {

	if err := videoList.Parse(); err != nil {
		return err
	}

	for _, vm := range videoList {

		fmt.Printf("merging videos with ID \"%s\"...", vm.Id)

		if err := vm.merge(); err != nil {
			fmt.Println("error!")
			log.Warnf("%v", err)

		} else {
			fmt.Println("done!")
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

	if root.Rename != nil {
		// Rename all video files in directory.
		if err := rename(); err != nil {
			log.Errorf("%v", err)
			return
		}
	}

	if root.Merge != nil {
		// Merge all video files that have matching IDs in directory.
		if err := merge(); err != nil {
			log.Errorf("%v", err)
			return
		}
	}

}
