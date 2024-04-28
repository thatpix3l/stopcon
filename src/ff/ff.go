package ff

type StreamVideo struct {
	Profile            string `json:"profile"`
	Width              int    `json:"width,omitempty"`
	Height             int    `json:"height,omitempty"`
	CodedWidth         int    `json:"coded_width,omitempty"`
	CodedHeight        int    `json:"coded_height,omitempty"`
	ClosedCaptions     int    `json:"closed_captions,omitempty"`
	FilmGrain          int    `json:"film_grain,omitempty"`
	HasBFrames         int    `json:"has_b_frames,omitempty"`
	SampleAspectRatio  string `json:"sample_aspect_ratio,omitempty"`
	DisplayAspectRatio string `json:"display_aspect_ratio,omitempty"`
	PixFmt             string `json:"pix_fmt,omitempty"`
	Level              int    `json:"level,omitempty"`
	ColorRange         string `json:"color_range,omitempty"`
	ColorSpace         string `json:"color_space,omitempty"`
	ColorTransfer      string `json:"color_transfer,omitempty"`
	ColorPrimaries     string `json:"color_primaries,omitempty"`
	ChromaLocation     string `json:"chroma_location,omitempty"`
	Refs               int    `json:"refs,omitempty"`
	RFrameRate         string `json:"r_frame_rate"`
	AvgFrameRate       string `json:"avg_frame_rate"`
	StartPts           int    `json:"start_pts"`
}

type Disposition struct {
	Default         int `json:"default"`
	Dub             int `json:"dub"`
	Original        int `json:"original"`
	Comment         int `json:"comment"`
	Lyrics          int `json:"lyrics"`
	Karaoke         int `json:"karaoke"`
	Forced          int `json:"forced"`
	HearingImpaired int `json:"hearing_impaired"`
	VisualImpaired  int `json:"visual_impaired"`
	CleanEffects    int `json:"clean_effects"`
	AttachedPic     int `json:"attached_pic"`
	TimedThumbnails int `json:"timed_thumbnails"`
	NonDiegetic     int `json:"non_diegetic"`
	Captions        int `json:"captions"`
	Descriptions    int `json:"descriptions"`
	Metadata        int `json:"metadata"`
	Dependent       int `json:"dependent"`
	StillImage      int `json:"still_image"`
}

type Stream struct {
	*StreamVideo
	Index          int         `json:"index"`
	CodecName      string      `json:"codec_name"`
	CodecLongName  string      `json:"codec_long_name"`
	CodecType      string      `json:"codec_type"`
	CodecTagString string      `json:"codec_tag_string"`
	CodecTag       string      `json:"codec_tag"`
	Disposition    Disposition `json:"disposition"`

	TimeBase      string `json:"time_base"`
	StartTime     string `json:"start_time"`
	ExtradataSize int    `json:"extradata_size"`

	Tags           map[string]interface{} `json:"tags,omitempty"`
	SampleFormat   string                 `json:"sample_fmt,omitempty"`
	SampleRate     string                 `json:"sample_rate,omitempty"`
	Channels       int                    `json:"channels,omitempty"`
	ChannelLayout  string                 `json:"channel_layout,omitempty"`
	BitsPerSample  int                    `json:"bits_per_sample,omitempty"`
	InitialPadding int                    `json:"initial_padding,omitempty"`
}

type Format struct {
	Filename       string                 `json:"filename"`
	NbStreams      int                    `json:"nb_streams"`
	NbPrograms     int                    `json:"nb_programs"`
	NbStreamGroups int                    `json:"nb_stream_groups"`
	FormatName     string                 `json:"format_name"`
	FormatLongName string                 `json:"format_long_name"`
	StartTime      string                 `json:"start_time"`
	Duration       string                 `json:"duration"`
	Size           string                 `json:"size"`
	BitRate        string                 `json:"bit_rate"`
	ProbeScore     int                    `json:"probe_score"`
	Tags           map[string]interface{} `json:"tags"`
}

type ProbeData struct {
	// Programs     []any    `json:"programs"`
	// StreamGroups []any    `json:"stream_groups"`
	Streams []Stream `json:"streams"`
	Format  Format   `json:"format"`
}
