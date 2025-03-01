package main

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

type execCmd struct {
	src  []string
	args []string
}

type goEnv struct {
	proxy        string
	proxyPresent bool
}

const pathSplitter = ","
const goProxy = "GOPROXY"

func main() {
	defer checkPanic()

	cmd := getExecCommand()
	outDir := prepareOutputDir(cmd)
	defer os.RemoveAll(outDir)
	exe := compileSources(cmd, outDir)
	execCommand(cmd, exe)

	panic("not reachable")
}

func getExecCommand() (cmd execCmd) {
	var err error

	if len(os.Args) <= 1 {
		panic("No file(s) to compile")
	}
	cmd.src = strings.Split(os.Args[1], pathSplitter)
	for i, _ := range cmd.src {
		cmd.src[i], err = filepath.Abs(cmd.src[i])
		panicIfError(err)
	}
	slices.Sort(cmd.src)
	cmd.args = os.Args[2:]
	return
}

func prepareOutputDir(cmd execCmd) (outDir string) {
	euid := os.Geteuid()
	outDir = filepath.Join(
		os.TempDir(),
		"gorun-"+strconv.Itoa(euid),
		hashStrings(cmd.src))
	stat, err := os.Stat(outDir)
	if err == nil {
		/* Check permissions */
		if !stat.IsDir() || stat.Mode().Perm() != 0o700 {
			panic("Output path is not a directory or has wrong permissions")
		}
	} else {
		err = os.MkdirAll(outDir, 0o700)
		panicIfError(err)
	}
	return
}

func compileSources(cmd execCmd, outDir string) (exe string) {
	hash := hashFiles(cmd.src)
	exe = filepath.Join(outDir, hash)
	_, err := os.Stat(exe)
	if err == nil {
		/* No need to compile */
		return
	}

	clearDir(outDir)
	defer clearSources(cmd, outDir)
	copySources(cmd, outDir)
	doCompileSources(cmd, outDir, exe)
	return
}

func execCommand(cmd execCmd, exe string) {
	var args = append([]string{exe}, cmd.args...)
	err := syscall.Exec(exe, args, os.Environ())
	panicIfError(err)
}

func doCompileSources(cmd execCmd, outDir string, exe string) {
	curWD, err := os.Getwd()
	panicIfError(err)
	err = os.Chdir(outDir)
	panicIfError(err)
	defer os.Chdir(curWD)

	goEn := setGoEnv()
	defer unsetGoEnv(goEn)

	err = runCommandSilent("go", "mod", "init", "gorun")
	panicIfError(err)
	err = runCommand("go", "mod", "tidy")
	panicIfError(err)
	err = runCommand("go", "build",
		"-o", exe,
		"-ldflags", "-s -w",
		"-trimpath")
	panicIfError(err)
}

func setGoEnv() (env goEnv) {
	var cacheDir, err = runCommandOutput("go", "env", "GOMODCACHE")
	panicIfError(err)
	cacheDir = strings.TrimSpace(cacheDir)

	env.proxy, env.proxyPresent = os.LookupEnv(goProxy)
	err = os.Setenv(goProxy,
		"file://"+filepath.Join(cacheDir, "cache", "download"))
	panicIfError(err)
	return
}

func unsetGoEnv(env goEnv) {
	var err error
	if env.proxyPresent {
		err = os.Setenv(goProxy, env.proxy)
	} else {
		err = os.Unsetenv(goProxy)
	}
	logIfError(err)
}

func copySources(cmd execCmd, outDir string) {
	var err error
	for _, src := range cmd.src {
		err = copyFile(src, filepath.Join(outDir, filepath.Base(src)))
		panicIfError(err)
	}
}

func clearSources(cmd execCmd, outDir string) {
	for _, src := range cmd.src {
		os.Remove(filepath.Join(outDir, filepath.Base(src)))
	}
	os.Remove(filepath.Join(outDir, "go.mod"))
	os.Remove(filepath.Join(outDir, "go.sum"))
}

func clearDir(path string) {
	lsDir, err := os.ReadDir(path)
	panicIfError(err)
	for _, lsItem := range lsDir {
		name := lsItem.Name()
		if name == "." || name == ".." {
			continue
		}
		err = os.Remove(filepath.Join(path, name))
		logIfError(err)
	}
}

func hashStrings(str []string) string {
	hash := sha256.New()
	for _, s := range str {
		hash.Write([]byte(s))
	}
	return base64.URLEncoding.EncodeToString(hash.Sum(nil))
}

func hashFiles(files []string) string {
	hash := sha256.New()
	for _, file := range files {
		f, err := os.Open(file)
		panicIfError(err)
		_, err = io.Copy(hash, f)
		f.Close()
		panicIfError(err)
	}
	return base64.URLEncoding.EncodeToString(hash.Sum(nil))
}

func runCommand(cmd string, args ...string) error {
	var command = exec.Command(cmd, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func runCommandSilent(cmd string, args ...string) error {
	var command = exec.Command(cmd, args...)
	return command.Run()
}

func runCommandOutput(cmd string, args ...string) (string, error) {
	var command = exec.Command(cmd, args...)
	var out, err = command.Output()
	return string(out), err
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
}
