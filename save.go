package main

import (
	"errors"
	"github.com/kr/fs"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var cmdSave = &Command{
	Usage: "save [-copy] [packages]",
	Short: "list current dependencies to a file",
	Long: `
Save writes a list of the dependencies of the named packages along
with the exact source control revision of each dependency. Output
is a JSON document with the following structure:

    type Godeps struct {
    	ImportPath string
    	GoVersion  string   // Abridged output of 'go version'.
    	Packages   []string // Arguments to godep save, if any.
    	Deps       []struct {
    		ImportPath string
    		Comment    string // Tag or description of commit, if present.
    		Rev        string // VCS-specific commit ID.
    	}
    }

If flag -copy is given, the list is written to Godeps/Godeps.json,
and source code for all dependencies is copied into Godeps.

Otherwise, the list alone is written to file Godeps.

For more about specifying packages, see 'go help packages'.
`,
	Run: runSave,
}

var flagCopy bool

func init() {
	cmdSave.Flag.BoolVar(&flagCopy, "copy", false, "copy source code")
}

func runSave(cmd *Command, args []string) {
	// Remove Godeps before listing packages, so that args
	// such as ./... don't match anything in there.
	if err := os.RemoveAll("Godeps"); err != nil {
		log.Fatalln(err)
	}
	g := &Godeps{
		ImportPath: MustLoadPackages(".")[0].ImportPath,
		GoVersion:  mustGoVersion(),
	}
	if len(args) > 0 {
		g.Packages = args
	} else {
		args = []string{"."}
	}
	a := MustLoadPackages(args...)
	err := g.Load(a)
	if err != nil {
		log.Fatalln(err)
	}
	if g.Deps == nil {
		g.Deps = make([]Dependency, 0) // produce json [], not null
	}
	manifest := "Godeps"
	if flagCopy {
		manifest = filepath.Join("Godeps", "Godeps.json")
		// We use a name starting with "_" so the go tool
		// ignores this directory when traversing packages
		// starting at the project's root. For example,
		//   godep go list ./...
		workspace := filepath.Join("Godeps", "_workspace")
		err = copySrc(workspace, g)
		if err != nil {
			log.Fatalln(err)
		}
		path := filepath.Join("Godeps", "Readme")
		err = ioutil.WriteFile(path, []byte(strings.TrimSpace(Readme)+"\n"), 0666)
		if err != nil {
			log.Println(err)
		}
		writeVCSIgnore(workspace)
	}
	f, err := os.Create(manifest)
	if err != nil {
		log.Fatalln(err)
	}
	_, err = g.WriteTo(f)
	if err != nil {
		log.Fatalln(err)
	}
	err = f.Close()
	if err != nil {
		log.Fatalln(err)
	}
}

func copySrc(dir string, g *Godeps) error {
	ok := true
	for _, dep := range g.Deps {
		w := fs.Walk(dep.pkg.Dir)
		for w.Step() {
			if w.Err() != nil {
				log.Println(w.Err())
				ok = false
				continue
			}
			if s := w.Stat().Name(); s[0] == '.' || s[1] == '_' {
				// Skip directories using a rule similar to how
				// the go tool enumerates packages.
				// See $GOROOT/src/cmd/go/main.go:/matchPackagesInFs
				w.SkipDir()
			}
			if w.Stat().IsDir() {
				continue
			}
			dst := filepath.Join(dir, w.Path()[len(dep.pkg.Root)+1:])
			if err := copyFile(dst, w.Path()); err != nil {
				log.Println(err)
				ok = false
			}
		}
	}
	if !ok {
		return errors.New("error copying source code")
	}
	return nil
}

// copyFile copies a regular file from src to dst.
// dst is opened with os.Create.
func copyFile(dst, src string) error {
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()
	err = os.MkdirAll(filepath.Dir(dst), 0777)
	if err != nil {
		return err
	}
	w, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	err1 := w.Close()
	if err == nil {
		err = err1
	}
	return err
}

// Func writeVCSIgnore writes "ignore" files inside dir for known VCSs,
// so that dir/pkg and dir/bin don't accidentally get committed.
// It logs any errors it encounters.
func writeVCSIgnore(dir string) {
	// Currently git is the only VCS for which we know how to do this.
	// Mercurial and Bazaar have similar mechasims, but they apparently
	// require writing files outside of dir.
	const ignore = "/pkg\n/bin\n"
	name := filepath.Join(dir, ".gitignore")
	err := ioutil.WriteFile(name, []byte(ignore), 0666)
	if err != nil {
		log.Println(err)
	}
}

const Readme = `
This directory tree is generated automatically by godep.

Please do not edit.

See https://github.com/kr/godep for more information.
`
