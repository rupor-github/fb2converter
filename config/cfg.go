// Package config abstracts all program configuration.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/asaskevich/govalidator"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"

	"fb2converter/go-micro/config"
	"fb2converter/go-micro/config/encoder"
	jsonenc "fb2converter/go-micro/config/encoder/json"
	"fb2converter/go-micro/config/encoder/toml"
	"fb2converter/go-micro/config/encoder/yaml"
	"fb2converter/go-micro/config/source"
	"fb2converter/go-micro/config/source/file"
	"fb2converter/go-micro/config/source/memory"
	"fb2converter/reporter"
)

//  Internal constants defining if program was invoked via MyHomeLib wrappers.
const (
	MhlNone int = iota
	MhlEpub
	MhlMobi
	MhlUnknown
)

// Logger configuration for single logger.
type Logger struct {
	Level       string `json:"level"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
}

// Fb2Mobi provides special support for MyHomeLib.
type Fb2Mobi struct {
	OutputFormat string `json:"output_format"`
}

// Fb2Epub provides special support for MyHomeLib.
type Fb2Epub struct {
	OutputFormat string `json:"output_format"`
	SendToKindle bool   `json:"send_to_kindle"`
}

// SMTPConfig keeps STK configuration.
type SMTPConfig struct {
	DeleteOnSuccess bool   `json:"delete_sent_book"`
	Server          string `json:"smtp_server"`
	Port            int    `json:"smtp_port"`
	User            string `json:"smtp_user"`
	Password        string `json:"smtp_password"`
	From            string `json:"from_mail"`
	To              string `json:"to_mail"`
}

// AuthorName is parsed author name from book metainfo.
type AuthorName struct {
	First  string `json:"first_name"`
	Middle string `json:"middle_name"`
	Last   string `json:"last_name"`
}

func (a *AuthorName) String() string {
	var res string
	if len(a.First) > 0 {
		res = a.First
	}
	if len(a.Middle) > 0 {
		res += " " + a.Middle
	}
	if len(a.Last) > 0 {
		res += " " + a.Last
	}
	return res
}

// MetaInfo keeps book meta-info overwrites from configuration.
type MetaInfo struct {
	ID         string        `json:"id"`
	ASIN       string        `json:"asin"`
	Title      string        `json:"title"`
	Lang       string        `json:"language"`
	Genres     []string      `json:"genres"`
	Authors    []*AuthorName `json:"authors"`
	SeqName    string        `json:"sequence"`
	SeqNum     int           `json:"sequence_number"`
	Date       string        `json:"date"`
	CoverImage string        `json:"cover_image"`
}

type confMetaOverwrite struct {
	Name string   `json:"name"`
	Meta MetaInfo `json:"meta"`
}

// IsValid checks if we have enough smtp parameters to attempt sending mail.
// It does not attempt actual connection.
func (c *SMTPConfig) IsValid() bool {
	return len(c.Server) > 0 && govalidator.IsDNSName(c.Server) &&
		c.Port > 0 && c.Port <= 65535 &&
		len(c.User) > 0 &&
		len(c.From) > 0 && govalidator.IsEmail(c.From) &&
		len(c.To) > 0 && govalidator.IsEmail(c.To)
}

// Doc format configuration for book processor.
type Doc struct {
	TitleFormat           string   `json:"title_format"`
	AuthorFormat          string   `json:"author_format"`
	AuthorFormatMeta      string   `json:"author_format_meta"`
	AuthorFormatFileName  string   `json:"author_format_file_name"`
	TransliterateMeta     bool     `json:"transliterate_meta"`
	OpenFromCover         bool     `json:"open_from_cover"`
	ChapterPerFile        bool     `json:"chapter_per_file"`
	ChapterLevel          int      `json:"chapter_level"`
	SeqNumPos             int      `json:"series_number_positions"`
	RemovePNGTransparency bool     `json:"remove_png_transparency"`
	ImagesScaleFactor     float64  `json:"images_scale_factor"`
	Stylesheet            string   `json:"style"`
	CharsPerPage          int      `json:"characters_per_page"`
	PagesPerFile          int      `json:"pages_per_file"`
	ChapterDividers       []string `json:"chapter_subtitle_dividers"`
	Hyphenate             bool     `json:"insert_soft_hyphen"`
	NoNBSP                bool     `json:"ignore_nonbreakable_space"`
	UseBrokenImages       bool     `json:"use_broken_images"`
	FileNameFormat        string   `json:"file_name_format"`
	FileNameTransliterate bool     `json:"file_name_transliterate"`
	FixZip                bool     `json:"fix_zip_format"`
	//
	DropCaps struct {
		Create        bool   `json:"create"`
		IgnoreSymbols string `json:"ignore_symbols"`
	} `json:"dropcaps"`
	Notes struct {
		BodyNames []string `json:"body_names"`
		Mode      string   `json:"mode"`
		Renumber  bool     `json:"renumber"`
		Format    string   `json:"link_format"`
	} `json:"notes"`
	Annotation struct {
		Create   bool   `json:"create"`
		AddToToc bool   `json:"add_to_toc"`
		Title    string `json:"title"`
	} `json:"annotation"`
	TOC struct {
		Type              string `json:"type"`
		Title             string `json:"page_title"`
		Placement         string `json:"page_placement"`
		MaxLevel          int    `json:"page_maxlevel"`
		NoTitleChapters   bool   `json:"include_chapters_without_title"`
		BookTitleFromMeta bool   `json:"book_title_from_meta"`
	} `json:"toc"`
	Cover struct {
		Default   bool   `json:"default"`
		ImagePath string `json:"image_path"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Resize    string `json:"resize"`
		Placement string `json:"stamp_placement"`
		Font      string `json:"stamp_font"`
	} `json:"cover"`
	Vignettes struct {
		Create bool                         `json:"create"`
		Images map[string]map[string]string `json:"images"`
	} `json:"vignettes"`
	//
	Transformations map[string]map[string]string `json:"transform"`
	//
	Kindlegen struct {
		Path             string `json:"path"`
		CompressionLevel int    `json:"compression_level"`
		Verbose          bool   `json:"verbose"`
		NoOptimization   bool   `json:"no_mobi_optimization"`
		RemovePersonal   bool   `json:"remove_personal_label"`
		PageMap          string `json:"generate_apnx"`
		ForceASIN        bool   `json:"force_asin_on_azw3"`
	} `json:"kindlegen"`
}

// names of supported vignettes
const (
	VigBeforeTitle = "before_title"
	VigAfterTitle  = "after_title"
	VigChapterEnd  = "chapter_end"
)

// Config keeps all configuration values.
type Config struct {
	// Internal implementation - keep it local, could be replaced
	Path string
	cfg  config.Config

	// Actual configuration used everywhere - immutable
	ConsoleLogger Logger
	FileLogger    Logger
	Doc           Doc
	SMTPConfig    SMTPConfig
	Fb2Mobi       Fb2Mobi
	Fb2Epub       Fb2Epub
	Overwrites    map[string]MetaInfo
}

var defaultConfig = []byte(`{
  "document": {
    "title_format": "{(#ABBRseries{ - #padnumber}) }#title",
    "author_format": "#l{ #f}{ #m}",
    "chapter_per_file": true,
    "chapter_level": 2147483647,
    "series_number_positions": 2,
    "characters_per_page": 2300,
    "pages_per_file": 2147483647,
    "fix_zip_format": true,
    "dropcaps": {
      "ignore_symbols": "'\"-.…0123456789‒–—«»“”\u003c\u003e"
    },
    "vignettes": {
      "create": true,
      "images": {
        "default": {
          "after_title": "profiles/vignettes/title_after.png",
          "before_title": "profiles/vignettes/title_before.png",
          "chapter_end": "profiles/vignettes/chapter_end.png"
        },
        "h0": {
          "after_title": "none",
          "before_title": "none",
          "chapter_end": "none"
        }
      }
    },
    "kindlegen": {
      "compression_level": 1,
      "remove_personal_label": true,
      "generate_apnx": "none"
    },
    "cover": {
      "height": 1680,
      "width": 1264
    },
    "notes": {
      "body_names": [ "notes", "comments" ],
      "mode": "default",
      "link_format": "[{#body_number.}#number]"
    },
    "annotation": {
      "title": "Annotation"
    },
    "toc": {
      "type": "normal",
      "page_title": "Content",
      "page_placement": "after",
      "page_maxlevel": 2147483647
    }
  },
  "logger": {
    "console": {
      "level": "normal"
    },
    "file": {
      "destination": "conversion.log",
      "level": "debug",
      "mode": "append"
    }
  },
  "fb2mobi": {
    "output_format": "mobi"
  },
  "fb2epub": {
    "output_format": "epub"
  }
}`)

// BuildConfig loads configuration.
func BuildConfig(fnames ...string) (*Config, error) {

	var err error
	// base configuration directory, always calculated from the path of the first configuration file
	var base string

	var configSources = []source.Source{
		memory.NewSource(memory.WithJSON(defaultConfig)),
	}

	var wasStdin bool
	for i, fname := range fnames {
		switch {
		case fname == "-":
			// NOTE: only one configuration could be read from STDIN, the rest should be ignored
			// Since logging is not yet setup at this point - we cannot even properly report it
			if !wasStdin {
				wasStdin = true
				// stdin - json format ONLY
				s, err := io.ReadAll(os.Stdin)
				if err != nil {
					return nil, fmt.Errorf("unable to read configuration from stdin: %w", err)
				}
				configSources = append(configSources, memory.NewSource(memory.WithJSON(s)))
				if i == 0 {
					if base, err = os.Getwd(); err != nil {
						return nil, fmt.Errorf("unable to get working directory: %w", err)
					}
				}
			}
		case len(fname) > 0:
			// from file
			var enc encoder.Encoder
			switch strings.ToLower(filepath.Ext(fname)) {
			case ".yml":
				fallthrough
			case ".yaml":
				enc = yaml.NewEncoder()
			case ".toml":
				enc = toml.NewEncoder()
			default:
				enc = jsonenc.NewEncoder()
			}
			configSources = append(configSources, file.NewSource(file.WithPath(fname), source.WithEncoder(enc)))
			if i == 0 {
				if base, err = filepath.Abs(filepath.Dir(fname)); err != nil {
					return nil, fmt.Errorf("unable to get configuration directory: %w", err)
				}
			}
		}
	}

	c := config.NewConfig()

	if err = c.Load(configSources...); err != nil {
		return nil, fmt.Errorf("unable to parse configuration %v", fnames)
	}

	conf := Config{cfg: c, Path: base, Overwrites: make(map[string]MetaInfo)}
	if err := c.Get("logger", "console").Scan(&conf.ConsoleLogger); err != nil {
		return nil, fmt.Errorf("unable to read console logger configuration: %w", err)
	}
	if err := c.Get("logger", "file").Scan(&conf.FileLogger); err != nil {
		return nil, fmt.Errorf("unable to read file logger configuration: %w", err)
	}
	if err := c.Get("document").Scan(&conf.Doc); err != nil {
		return nil, fmt.Errorf("unable to read document format configuration: %w", err)
	}
	if err := c.Get("fb2mobi").Scan(&conf.Fb2Mobi); err != nil {
		return nil, fmt.Errorf("unable to read fb2mobi cnfiguration: %w", err)
	}
	if err := c.Get("fb2epub").Scan(&conf.Fb2Epub); err != nil {
		return nil, fmt.Errorf("unable to read fb2epub cnfiguration: %w", err)
	}
	if err := c.Get("sendtokindle").Scan(&conf.SMTPConfig); err != nil {
		return nil, fmt.Errorf("unable to read send to kindle cnfiguration: %w", err)
	}

	var metas []confMetaOverwrite
	if err := c.Get("overwrites").Scan(&metas); err != nil {
		return nil, fmt.Errorf("unable to read meta information overwrites: %w", err)
	}
	for _, meta := range metas {
		name := filepath.ToSlash(meta.Name)
		if _, exists := conf.Overwrites[name]; !exists {
			conf.Overwrites[name] = meta.Meta
		}
	}

	// some defaults
	if conf.Doc.Kindlegen.CompressionLevel < 0 || conf.Doc.Kindlegen.CompressionLevel > 2 {
		conf.Doc.Kindlegen.CompressionLevel = 1
	}
	// to keep old behavior
	if len(conf.Doc.AuthorFormatMeta) == 0 {
		conf.Doc.AuthorFormatMeta = conf.Doc.AuthorFormat
	}
	if len(conf.Doc.AuthorFormatFileName) == 0 {
		conf.Doc.AuthorFormatFileName = conf.Doc.AuthorFormat
	}
	return &conf, nil
}

// GetBytes returns configuration the way it was read from various sources, before unmarshaling.
func (conf *Config) GetBytes() ([]byte, error) {
	// do some pretty-printing
	var out bytes.Buffer
	err := json.Indent(&out, conf.cfg.Bytes(), "", "  ")
	return out.Bytes(), err
}

// Transformation is used to specify additional text processsing during conversion.
type Transformation struct {
	From string
	To   string
}

// GetTransformation returns pointer to named text transformation of nil if none eavailable.
func (conf *Config) GetTransformation(name string) *Transformation {

	if len(conf.Doc.Transformations) == 0 {
		return nil
	}

	m, exists := conf.Doc.Transformations[name]
	if !exists {
		return nil
	}

	if f, ok := m["from"]; ok && len(f) > 0 {
		return &Transformation{
			From: f,
			To:   m["to"],
		}
	}
	return nil
}

// GetOverwrite returns pointer to information to be used instead of parsed data.
func (conf *Config) GetOverwrite(name string) *MetaInfo {

	if len(conf.Overwrites) == 0 {
		return nil
	}

	// start from most specific

	// NOTE: all path separators were converted to slash before being added to map
	name = filepath.ToSlash(name)
	for {
		if i, ok := conf.Overwrites[name]; ok {
			return &i
		}
		parts := strings.SplitN(name, "/", 1)
		if len(parts) <= 1 {
			break
		}
		name = parts[1]
	}

	// not found - see if we have generic overwrite
	name = "*"
	if i, ok := conf.Overwrites[name]; ok {
		return &i
	}
	return nil
}

// GetKindlegenPath provides platform specific path to the kindlegen executable.
func (conf *Config) GetKindlegenPath() (string, error) {

	fname := conf.Doc.Kindlegen.Path
	expath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("unable to detect program path: %w", err)
	}
	if expath, err = filepath.Abs(filepath.Dir(expath)); err != nil {
		return "", fmt.Errorf("unable to calculate program path: %w", err)
	}

	if len(fname) > 0 {
		if !filepath.IsAbs(fname) {
			fname = filepath.Join(expath, fname)
		}
	} else {
		fname = filepath.Join(expath, kindlegen())
	}
	if _, err = os.Stat(fname); err != nil {
		return "", fmt.Errorf("unable to find kindlegen: %w", err)
	}
	return fname, nil
}

// GetActualBytes returns actual configuration, including fields initialized by default.
func (conf *Config) GetActualBytes() ([]byte, error) {

	// For convinience create temporary configuration structure with actual values
	a := struct {
		B struct {
			Cl Logger `json:"console"`
			Fl Logger `json:"file"`
		} `json:"logger"`
		D Doc        `json:"document"`
		E SMTPConfig `json:"sendtokindle"`
		F Fb2Mobi    `json:"fb2mobi"`
		G Fb2Epub    `json:"fb2epub"`
		H []struct {
			Name string   `json:"name"`
			Meta MetaInfo `json:"meta"`
		} `json:"overwrites"`
	}{}
	a.B.Cl = conf.ConsoleLogger
	a.B.Fl = conf.FileLogger
	a.D = conf.Doc
	a.E = conf.SMTPConfig
	a.F = conf.Fb2Mobi
	a.G = conf.Fb2Epub

	for k, v := range conf.Overwrites {
		s := struct {
			Name string   `json:"name"`
			Meta MetaInfo `json:"meta"`
		}{Name: filepath.FromSlash(k), Meta: v}
		a.H = append(a.H, s)
	}

	// Marshall it to json
	b, err := json.Marshal(a)
	if err != nil {
		return []byte{}, err
	}

	// And pretty-print it
	var out bytes.Buffer
	err = json.Indent(&out, b, "", "  ")
	return out.Bytes(), err
}

// PrepareLog returns our standard logger. It prepares zap logger for use by the program.
func (conf *Config) PrepareLog(rpt *reporter.Report) (*zap.Logger, error) {

	// Console - split stdout and stderr, handle colors and redirection

	ec := zap.NewDevelopmentEncoderConfig()
	ec.EncodeCaller = nil
	if EnableColorOutput(os.Stdout) {
		ec.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		ec.EncodeLevel = zapcore.CapitalLevelEncoder
	}
	consoleEncoderLP := zapcore.NewConsoleEncoder(ec)

	ec = zap.NewDevelopmentEncoderConfig()
	ec.EncodeCaller = nil
	if EnableColorOutput(os.Stderr) {
		ec.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		ec.EncodeLevel = zapcore.CapitalLevelEncoder
	}
	consoleEncoderHP := newEncoder(ec) // filter errorVerbose

	highPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})

	var consoleCoreHP, consoleCoreLP zapcore.Core
	switch conf.ConsoleLogger.Level {
	case "normal":
		consoleCoreLP = zapcore.NewCore(consoleEncoderLP, zapcore.Lock(os.Stdout),
			zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
				return zapcore.InfoLevel <= lvl && lvl < zapcore.ErrorLevel
			}))
		consoleCoreHP = zapcore.NewCore(consoleEncoderHP, zapcore.Lock(os.Stderr), highPriority)
	case "debug":
		consoleCoreLP = zapcore.NewCore(consoleEncoderLP, zapcore.Lock(os.Stdout),
			zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
				return zapcore.DebugLevel <= lvl && lvl < zapcore.ErrorLevel
			}))
		consoleCoreHP = zapcore.NewCore(consoleEncoderHP, zapcore.Lock(os.Stderr), highPriority)
	default:
		consoleCoreLP = zapcore.NewNopCore()
		consoleCoreHP = zapcore.NewNopCore()
	}

	// File

	opener := func(fname, mode string) (f *os.File, err error) {
		flags := os.O_CREATE | os.O_WRONLY
		if mode == "append" {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		if f, err = os.OpenFile(fname, flags, 0644); err != nil {
			return nil, err
		}
		return f, nil
	}

	var (
		fileEncoder    zapcore.Encoder
		fileCore       zapcore.Core
		logLevel       zap.AtomicLevel
		logRequested   bool
		levelRequested = conf.FileLogger.Level
		modeRequested  = conf.FileLogger.Mode
	)

	if rpt != nil {
		// if report is requested always set maximum available logging level for file logger
		levelRequested = "debug"
		modeRequested = "overwrite"
	}

	switch levelRequested {
	case "debug":
		fileEncoder = zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
		logLevel = zap.NewAtomicLevelAt(zap.DebugLevel)
		logRequested = true
	case "normal":
		fileEncoder = zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
		logLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
		logRequested = true
	}

	var newName string
	if logRequested {
		if f, err := opener(conf.FileLogger.Destination, modeRequested); err == nil {
			fileCore = zapcore.NewCore(fileEncoder, zapcore.Lock(f), logLevel)
			rpt.Store("file.log", f.Name())
		} else if f, err = os.CreateTemp("", "conversion.*.log"); err == nil {
			newName = f.Name()
			fileCore = zapcore.NewCore(fileEncoder, zapcore.Lock(f), logLevel)
			rpt.Store("file.log", newName)
		} else {
			return nil, fmt.Errorf("unable to access file log destination (%s): %w", conf.FileLogger.Destination, err)
		}
	} else {
		fileCore = zapcore.NewNopCore()
	}

	core := zap.New(zapcore.NewTee(consoleCoreHP, consoleCoreLP, fileCore), zap.AddCaller())
	if len(newName) != 0 {
		// log was redirected - we need to report this
		core.Warn("Log file was redirected to new location", zap.String("location", newName))
	}
	return core, nil
}

// When logging error to console - do not output verbose message.

type consoleEnc struct {
	zapcore.Encoder
}

func newEncoder(cfg zapcore.EncoderConfig) zapcore.Encoder {
	return consoleEnc{zapcore.NewConsoleEncoder(cfg)}
}

func (c consoleEnc) Clone() zapcore.Encoder {
	return consoleEnc{c.Encoder.Clone()}
}

func (c consoleEnc) EncodeEntry(ent zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	var newFields []zapcore.Field
	for _, f := range fields {
		if f.Type == zapcore.ErrorType {
			// presently superficial - but we may need to shorten what is printed to console in the future
			e := f.Interface.(error)
			f.Interface = errors.New(e.Error())
		}
		newFields = append(newFields, f)
	}
	return c.Encoder.EncodeEntry(ent, newFields)
}
