package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"fb2converter/config"
	"fb2converter/misc"
)

// params := Format('"%s" "%s"', [InpFile, ChangeFileExt(OutFile, '.epub')]);
// Result := ExecAndWait(FAppPath + 'converters\fb2epub\fb2epub.exe', params, SW_HIDE);

func main() {

	log.SetPrefix("\n*** ")

	usage := func() {
		fmt.Fprintf(os.Stderr, "\nMyHomeLib wrapper for fb2 converter\nVersion %s (%s) : %s\n\n",
			misc.GetVersion(), runtime.Version(), misc.GetGitHash())
		fmt.Fprintf(os.Stderr, "Usage: %s <from fb2> <to epub>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       Supports configuration file \"./fb2epub.toml\"\n\n")
	}

	if len(os.Args) < 3 {
		usage()
		os.Exit(0)
	}

	expath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}

	converter := config.FindConverter(expath)
	if len(converter) == 0 {
		log.Fatal("Unable to locate converter engine")
	}

	from, err := filepath.Abs(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(from); err != nil {
		log.Fatal("Source file does not exist", from, err)
	}

	to, err := filepath.Abs(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	to = filepath.Dir(to)
	if _, err := os.Stat(to); err != nil {
		log.Fatal("Destination directory does not exist", to, err)
	}

	args := make([]string, 0, 10)
	args = append(args, "-mhl", fmt.Sprintf("%d", config.MhlEpub))

	config := filepath.Join(filepath.Dir(expath), "fb2epub.toml")
	if _, err := os.Stat(config); err == nil {
		args = append(args, "-config", config)
	}

	args = append(args, "convert")
	args = append(args, "--ow")

	args = append(args, from)
	args = append(args, to)

	cmd := exec.Command(converter, args...)

	fmt.Printf("Starting %s with %q\n", converter, args)

	out, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal("Unable to redirect converter output", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatal("Unable to start converter", err)
	}

	// read and print converter stdout
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Fatal("Converter stdout pipe broken", err)
	}

	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			log.Println(string(ee.Stderr))
		}
		log.Fatal("Converter returned error", err)
	}
}
