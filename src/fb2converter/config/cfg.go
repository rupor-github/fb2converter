package config

import (
	"bytes"
	"encoding/json"
	goerr "errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/micro/go-config"
	"github.com/micro/go-config/encoder"
	jsonenc "github.com/micro/go-config/encoder/json"
	"github.com/micro/go-config/encoder/toml"
	"github.com/micro/go-config/encoder/yaml"
	"github.com/micro/go-config/source"
	"github.com/micro/go-config/source/file"
	"github.com/micro/go-config/source/memory"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
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

// IsValid checks if we have enough smtp parameters to attemp sending mail.
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
	TitleFormat           string  `json:"title_format"`
	AuthorFormat          string  `json:"author_format"`
	TransliterateMeta     bool    `json:"transliterate_meta"`
	OpenFromCover         bool    `json:"open_from_cover"`
	ChapterPerFile        bool    `json:"chapter_per_file"`
	ChapterLevel          int     `json:"chapter_level"`
	SeqNumPos             int     `json:"series_number_positions"`
	RemovePNGTransparency bool    `json:"remove_png_transparency"`
	ImagesScaleFactor     float64 `json:"images_scale_factor"`
	Stylesheet            string  `json:"style"`
	CharsPerPage          int     `json:"characters_per_page"`
	Hyphenate             bool    `json:"insert_soft_hyphen"`
	UseBrokenImages       bool    `json:"use_broken_images"`
	FileNameFormat        string  `json:"file_name_format"`
	//
	DropCaps struct {
		Create        bool   `json:"create"`
		IgnoreSymbols string `json:"ignore_symbols"`
	} `json:"dropcaps"`
	Notes struct {
		BodyNames []string `json:"body_names"`
		Mode      string   `json:"mode"`
	} `json:"notes"`
	Annotation struct {
		Create bool
		Title  string `json:"title"`
	} `json:"annotation"`
	TOC struct {
		Type      string `json:"type"`
		Title     string `json:"page_title"`
		Placement string `json:"page_placement"`
		MaxLevel  int    `json:"page_maxlevel"`
	} `json:"toc"`
	Cover struct {
		Default   bool   `json:"default"`
		ImagePath string `json:"image_path"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
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
}

var defaultConfig = []byte(`{
  "document": {
    "title_format": "{(#abbrseries{ #padnumber}) }#title",
    "author_format": "#l{ #f}{ #m}",
    "chapter_per_file": true,
    "chapter_level": 2147483647,
    "series_number_positions": 2,
    "characters_per_page": 2300,
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
      "mode": "default"
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
  }
}`)

// BuildConfig loads configuration from file.
func BuildConfig(fname string) (*Config, error) {

	var base string

	c := config.NewConfig()
	if fname == "-" {
		// stdin - json format ONLY
		source, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return nil, errors.Wrap(err, "unable to read configuration from stdin")
		}
		err = c.Load(
			// default values
			memory.NewSource(memory.WithData(defaultConfig)),
			// overwrite
			memory.NewSource(memory.WithData(source)))

		if err != nil {
			return nil, errors.Wrap(err, "unable to read configuration from stdin")
		}
		if base, err = os.Getwd(); err != nil {
			return nil, errors.Wrap(err, "unable to get working directory")
		}
	} else if len(fname) > 0 {
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
		err := c.Load(
			// default values
			memory.NewSource(memory.WithData(defaultConfig)),
			// overwrite
			file.NewSource(file.WithPath(fname), source.WithEncoder(enc)))

		if err != nil {
			return nil, errors.Wrapf(err, "unable to read configuration file (%s)", fname)
		}
		if base, err = filepath.Abs(filepath.Dir(fname)); err != nil {
			return nil, errors.Wrap(err, "unable to get configuration directory")
		}
	} else {
		// default values
		err := c.Load(memory.NewSource(memory.WithData(defaultConfig)))
		if err != nil {
			return nil, errors.Wrap(err, "unable to prepare default configuration")
		}
	}

	conf := Config{cfg: c, Path: base}
	if err := c.Get("logger", "console").Scan(&conf.ConsoleLogger); err != nil {
		return nil, errors.Wrap(err, "unable to read console logger configuration")
	}
	if err := c.Get("logger", "file").Scan(&conf.FileLogger); err != nil {
		return nil, errors.Wrap(err, "unable to read file logger configuration")
	}
	if err := c.Get("document").Scan(&conf.Doc); err != nil {
		return nil, errors.Wrap(err, "unable to read document format configuration")
	}
	if err := c.Get("fb2mobi").Scan(&conf.Fb2Mobi); err != nil {
		return nil, errors.Wrap(err, "unable to read fb2mobi cnfiguration")
	}
	if err := c.Get("sendtokindle").Scan(&conf.SMTPConfig); err != nil {
		return nil, errors.Wrap(err, "unable to read send to kindle cnfiguration")
	}
	// some defaults
	if conf.Doc.Kindlegen.CompressionLevel < 0 || conf.Doc.Kindlegen.CompressionLevel > 2 {
		conf.Doc.Kindlegen.CompressionLevel = 1
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

// GetKindlegenPath provides platform specific path to the kindlegen executable.
func (conf *Config) GetKindlegenPath() (string, error) {

	fname := conf.Doc.Kindlegen.Path
	expath, err := os.Executable()
	if err != nil {
		return "", errors.Wrap(err, "unable to detect program path")
	}
	if expath, err = filepath.Abs(filepath.Dir(expath)); err != nil {
		return "", errors.Wrap(err, "unable to calculate program path")
	}

	if len(fname) > 0 {
		if !filepath.IsAbs(fname) {
			fname = filepath.Join(expath, fname)
		}
	} else {
		fname = filepath.Join(expath, kindlegen())
	}
	if _, err = os.Stat(fname); err != nil {
		return "", errors.Wrap(err, "unable to find kindlegen")
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
	}{}
	a.B.Cl = conf.ConsoleLogger
	a.B.Fl = conf.FileLogger
	a.D = conf.Doc
	a.E = conf.SMTPConfig
	a.F = conf.Fb2Mobi

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
func (conf *Config) PrepareLog() (*zap.Logger, error) {

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

	ec = zap.NewDevelopmentEncoderConfig()
	fileEncoder := zapcore.NewConsoleEncoder(ec)

	opener := func(fname, mode string) (f *os.File, err error) {
		flags := os.O_CREATE | os.O_WRONLY
		if mode == "append" {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		if f, err = os.OpenFile(conf.FileLogger.Destination, flags, 0644); err != nil {
			return nil, err
		}
		return f, nil
	}

	var (
		fileCore     zapcore.Core
		logLevel     zap.AtomicLevel
		logRequested bool
	)
	switch conf.FileLogger.Level {
	case "debug":
		logLevel = zap.NewAtomicLevelAt(zap.DebugLevel)
		logRequested = true
	case "normal":
		logLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
		logRequested = true
	}

	if logRequested {
		if f, err := opener(conf.FileLogger.Destination, conf.FileLogger.Mode); err == nil {
			fileCore = zapcore.NewCore(fileEncoder, zapcore.Lock(f), logLevel)
		} else {
			return nil, errors.Wrapf(err, "unable to access file log destination (%s)", conf.FileLogger.Destination)
		}
	} else {
		fileCore = zapcore.NewNopCore()
	}

	return zap.New(zapcore.NewTee(consoleCoreHP, consoleCoreLP, fileCore), zap.AddCaller()), nil
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
			e := f.Interface.(error)
			f.Interface = goerr.New(e.Error())
		}
		newFields = append(newFields, f)
	}
	return c.Encoder.EncodeEntry(ent, newFields)
}
