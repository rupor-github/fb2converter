<p align="center">
    <h1 align="center">fb2converter</h1>
    <p align="center">
        converts fb2 files to epub, kepub, mobi, azw3
    </p>
    <p align="center">
        <a href="https://pkg.go.dev/mod/github.com/rupor-github/fb2converter/?tab=packages"><img alt="GoDoc" src="https://img.shields.io/badge/godoc-reference-blue.svg" /></a>
        <a href="https://goreportcard.com/report/github.com/rupor-github/fb2converter"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/rupor-github/fb2converter" /></a>
    </p>
    <hr>
</p>

### A complete rewrite of [fb2mobi](https://github.com/rupor-github/fb2mobi).
 
  Russian [WiKi](https://github.com/rupor-github/fb2converter/wiki/fb2converter)
  
  Russian [forum](https://4pda.ru/forum/index.php?showtopic=942250).

Aims to be faster than python implementation and much easier to maintain. Simpler configuration, zero dependencies,
better diagnostics and no installation required.

### Essential differences:

- no UI: use [Libro](https://github.com/dnkorpushov/libro) or [MyHomeLib](https://github.com/OleksiyPenkov/myhomelib)
- no XSL pre-processing (see document.transform configuration instead)
- no XML configuration - use [TOML](https://github.com/toml-lang/toml), [YAML](https://yaml.org/) or [JSON](https://www.json.org/) format instead
- no "default" external configuration, path to configuration file has to be supplied - always
- no overwriting of configuration parameters from command line, options either specified in configuration file or on command line
- slightly different hyphenation algorithm (no hyphensReplaceNBSP)
- fixes and echancements in toc.ncx generation
- epub processing was separated into its own command "transfer" and any attempts to process epub content were dropped
- go differs in how it processes images, it is less forgiving than Python's PILLOW and do not have lazy decoding (see use_broken_images configuration option)
- small changes in result formatting, for example:
  - chapter-end vignette would not be added if chapter does not have text paragraphs
  - html tags unknown to fb2 spec may be dropped depending on context
  - page size is calculated based on proper Unicode code points rather than byte size
  - ...
- full support for kepub format
- processing of files, directories, zip archives and directories with zip archives - no special consideration is made for `.fb2.zip` files.
- flexible output path/name formatting
- fb2c could be build for any platform supported by [go language](https://golang.org/doc/install). If mobi or azw3 are required additional limitations are imposed by [Amazon's kindlegen](https://www.amazon.com/gp/feature.html?ie=UTF8&docId=1000765211)
- fb2c has no dependencies and does not require installation or any kind

### Installation:

Download from the [releases page](https://github.com/rupor-github/fb2converter/releases) and unpack it in a convenient location.

Starting with v1.58.0 releases are packed with zip and signed with [minisign](https://jedisct1.github.io/minisign/). Here is public key for verification: ![key](doc/build_key.png) RWTNh1aN8DrXq26YRmWO3bPBx4m8jBATGXt4Z96DF4OVSzdCBmoAU+Vq

### Usage:

Configuration is fully documented [here](https://github.com/rupor-github/fb2converter/blob/master/static/configuration.toml).
In order to customize program behavior use "export" command to the directory of your choice and then supply path to your configuration file during program run.

Program has detailed logging configured by default (by default conversion.log in current working directory) - in case of problems, take a look there first.

```
>>> ./fb2c
NAME:
   fb2converter - fb2 conversion engine

USAGE:
   fb2c.exe [global options] command [command options] [arguments...]

VERSION:
   "program version" ("go runtime version") : "git sha string"

COMMANDS:
     convert     Converts FB2 file(s) to specified format
     transfer    Prepares EPUB file(s) for transfer (Kindle only!)
     synccovers  Extracts thumbnails from documents (Kindle only!)
     dumpconfig  Dumps active configuration (JSON)
     export      Exports built-in resources for customization
     help, h     Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --config FILE, -c FILE  load configuration from FILE (YAML, TOML or JSON). if FILE is "-" JSON will be expected from STDIN
   --debug, -d             leave behind various artifacts for debugging (do not delete intermediate results)
   --help, -h              show help
   --version, -v           print the version
```

Additional help for any command could be obtained by running `./fb2c help COMMAND-NAME`.

### Examples:

In order to convert all fb2 files in `c:\books\to-read` directory and get results in `d:\out` directory without keeping original subdirectory structure
sending mobi files to Kindle via e-mail in process execute

   `fb2c.exe convert --nodirs --stk --to mobi c:\books\to-read d:\out`

If you want resulting mobi files to be located alongside with original files, do something like

   `fb2c.exe convert --to mobi c:\books\to-read c:\books\to-read`

### MyHomeLib support:

Windows builds come with full [MyHomeLib](https://github.com/OleksiyPenkov/myhomelib) support. Just make sure that your `MyHomeLib\converters` directory does not contain old
`fb2mobi` and/or `fb2epub` subdirectories and unpack `fb2c_win32.zip` or `fb2c_win64.zip` there. It is a drop-in replacement and should be functional out of the box in most cases. 

#### NOTE:
* `fb2mobi.exe` looks for `fb2mobi.toml` in its directory (similarly `fb2epub.exe` looks for `fb2epub.toml`), so any additional customization is easy.
* __Do not install__ MyHomeLib in either `%ProgramFiles%` or `%ProgramFiles(x86)%` directory - it is bad idea. Since for regular user accounts In Windows those places are __write-protected__ you will have difficulties copying converters there and converters will have problems creating conversion logs which are enabled by default.
