//go:build tools
// +build tools

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"text/tabwriter"
)

var defaultLogger *Logger

func init() {
	defaultLogger = NewLogger()
}

const grammarsJson = "./_automation/grammars.json"

type GrammarVersion struct {
	Reference      string `json:"reference"`
	Revision       string `json:"revision"`
	UpdatedBasedOn string `json:"updateBasedOn"` //"tag" or "commit". If not defined, "commit" is default
}

type Grammar struct {
	Language string   `json:"language"`
	URL      string   `json:"url"`
	Files    []string `json:"files"`
	*GrammarVersion
}

func (g *Grammar) ContentURL() string {
	return fmt.Sprintf(
		"https://raw.githubusercontent.com/%s",
		strings.TrimPrefix(g.URL, "https://github.com/"),
	)
}

func (g *Grammar) FetchNewVersion() *GrammarVersion {
	if strings.EqualFold(g.UpdatedBasedOn, "tag") {
		tag, rev := fetchLastTag(g.URL)
		if tag != g.Reference {
			return &GrammarVersion{
				Reference: tag,
				Revision:  rev,
			}
		}
	} else {
		rev := fetchLastCommit(g.URL, g.Reference)
		if rev != g.Revision {
			return &GrammarVersion{
				Reference: g.Reference,
				Revision:  rev,
			}
		}
	}
	return nil
}

func root(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("sub-command expected")
	}

	ctx := context.Background()
	s := NewUpdateService()

	switch args[0] {
	case "check-updates":
		fs := flag.NewFlagSet("check-updates", flag.ExitOnError)
		flagsParse(fs, args[1:])

		s.CheckUpdates(ctx)

	case "update":
		if len(args) < 2 {
			return fmt.Errorf("language argument is missing")
		}

		fs := flag.NewFlagSet("update", flag.ExitOnError)
		force := fs.Bool("force", false, "re-download grammar even if revision is the same")
		flagsParse(fs, args[2:])

		updated := s.Update(ctx, args[1], *force)
		s.writeGrammarsFile(ctx)

		// A caller naming a single grammar asked for that grammar, and an exit
		// status alone would not otherwise distinguish a skip from a completed
		// update. update-all deliberately does not do this: csharp and php are
		// patched permanently, so failing the run on one of them would make a
		// non-zero exit its steady state.
		if !updated {
			return fmt.Errorf("%s was left untouched; see the warning above", args[1])
		}

	case "update-all":
		fs := flag.NewFlagSet("update-all", flag.ExitOnError)
		flagsParse(fs, args[1:])

		s.UpdateAll(ctx, true)
		s.writeGrammarsFile(ctx)

	default:
		return fmt.Errorf("unknown sub-command")
	}

	// Reported only now: grammars.json has been written, so the grammars that
	// did update are recorded before the run fails.
	if patchGuardTripped.Load() {
		return fmt.Errorf("a locally patched file was left untouched; see the error above")
	}

	return nil
}

func main() {
	if err := root(os.Args[1:]); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type UpdateService struct {
	grammars    []*Grammar
	grammarsMap map[string]*Grammar
}

func NewUpdateService() *UpdateService {
	b, err := os.ReadFile(grammarsJson)
	if err != nil {
		logAndExit(defaultLogger, err.Error(), "file", grammarsJson)
	}

	var grammars []*Grammar
	err = json.Unmarshal(b, &grammars)
	if err != nil {
		logAndExit(defaultLogger, fmt.Sprintf("unmarshaling grammars file error: %w", err), "file", grammarsJson)
	}

	grammarsMap := make(map[string]*Grammar, len(grammars))
	for _, g := range grammars {
		grammarsMap[g.Language] = g
	}

	return &UpdateService{grammars, grammarsMap}
}

func (s *UpdateService) CheckUpdates(ctx context.Context) {
	newVersions := s.fetchNewVersions()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	for i, g := range s.grammars {
		nextVersion := newVersions[i]
		fmt.Fprintf(w, "%s\t%s\t", g.Language, g.Reference)
		if nextVersion == nil {
			fmt.Fprintf(w, "(up-to-date)")
		} else if g.Reference == nextVersion.Reference {
			fmt.Fprintf(w, "(update available: %s -> %s)", g.Revision, nextVersion.Revision)
		} else {
			fmt.Fprintf(w, "(update available: %s -> %s)", g.Reference, nextVersion.Reference)
		}
		fmt.Fprintln(w)
	}
	w.Flush()
}

func (s *UpdateService) fetchNewVersions() []*GrammarVersion {
	results := make([]*GrammarVersion, len(s.grammars))
	wg := sync.WaitGroup{}
	wg.Add(len(s.grammars))

	for i, g := range s.grammars {
		go func(i int, g *Grammar) {
			defer wg.Done()
			results[i] = g.FetchNewVersion()
		}(i, g)
	}

	wg.Wait()

	return results
}

// Update reports whether the grammar's sources were re-downloaded. False means
// it was skipped because it carries local patches.
func (s *UpdateService) Update(ctx context.Context, language string, force bool) bool {
	logger := getLogger(ctx).With("language", language)

	grammar, ok := s.grammarsMap[language]
	if !ok {
		logAndExit(logger, "grammar not found")
	}

	logger = logger.With("reference", grammar.Reference)
	ctx = context.WithValue(ctx, loggerCtxKey, logger)

	v := grammar.FetchNewVersion()
	if v == nil {
		if !force {
			logAndExit(logger, "grammar is not outdated")
		} else {
			logger.Warn("re-downloading up-to-date grammar")
		}
	}

	// Set before downloading, never after. Every downloader interpolates
	// grammar.Revision into its URL, and that revision is a concrete commit
	// resolved by git ls-remote rather than a symbolic ref the server would
	// resolve to tip, so recording the new revision afterwards would re-fetch
	// the revision already vendored and then claim the new one.
	prevReference, prevRevision := grammar.Reference, grammar.Revision
	if v != nil {
		grammar.Reference = v.Reference
		grammar.Revision = v.Revision
	}

	if !s.downloadGrammar(ctx, grammar) {
		// Nothing was written, so put the pin back: recording a revision for
		// files that were left untouched would make grammars.json describe
		// sources that do not match it.
		grammar.Reference, grammar.Revision = prevReference, prevRevision
		return false
	}

	return true
}

func (s *UpdateService) UpdateAll(ctx context.Context, force bool) {
	wg := sync.WaitGroup{}
	for _, g := range s.grammars {
		wg.Add(1)

		go func(g *Grammar) {
			defer wg.Done()

			// Outcome ignored: a skipped grammar is reported by downloadGrammar
			// and must not stop the rest of the batch.
			_ = s.Update(ctx, g.Language, force)
		}(g)
	}
	wg.Wait()
}

// downloadGrammar reports whether the grammar was downloaded. False means it
// was skipped because it carries local patches; the caller must not record a
// new revision for it.
func (s *UpdateService) downloadGrammar(ctx context.Context, g *Grammar) bool {
	logger := getLogger(ctx).With("language", g.Language, "reference", g.Reference)
	logger.Info("downloading")

	ctx = context.WithValue(ctx, loggerCtxKey, logger)

	// Skip before any write. Aborting instead would kill the process from
	// inside an UpdateAll goroutine, so grammars.json would never be written
	// and every grammar that did re-download in the same run would be left on
	// disk with its old revision still recorded.
	if patched := locallyPatchedFiles(g.Language); len(patched) > 0 && !overwritePatchedAllowed() {
		logger.Warn(
			"skipping locally patched grammar: re-apply the patches described in its header "+
				"against the new upstream baseline, regenerate, and replace it by hand; "+
				"set "+allowOverwritePatchedEnv+"=1 to overwrite",
			"files", strings.Join(patched, ", "),
		)
		return false
	}

	s.makeDir(ctx, g.Language)

	switch g.Language {
	case "dockerfile":
		s.downloadDockerfile(ctx, g)
	case "ocaml":
		s.downloadOcaml(ctx, g)
	case "typescript":
		s.downloadTypescript(ctx, g)
	case "yaml":
		s.downloadYaml(ctx, g)
	case "php":
		s.downloadPhp(ctx, g)
	case "markdown":
		s.downloadMarkdown(ctx, g)
	case "sql":
		s.downloadSql(ctx, g)
	default:
		s.defaultGrammarDownload(ctx, g)
	}

	logger.Info("successfully downloaded")

	return true
}

func (s *UpdateService) makeDir(ctx context.Context, path string) {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		logAndExit(getLogger(ctx), err.Error(), "path", path)
	}
}

func (s *UpdateService) defaultGrammarDownload(ctx context.Context, g *Grammar) {
	url := g.ContentURL()

	s.downloadFile(
		ctx,
		fmt.Sprintf("%s/%s/src/tree_sitter/parser.h", url, g.Revision),
		fmt.Sprintf("%s/parser.h", g.Language),
		nil,
	)

	for _, f := range g.Files {
		s.downloadFile(
			ctx,
			fmt.Sprintf("%s/%s/src/%s", url, g.Revision, f),
			fmt.Sprintf("%s/%s", g.Language, f),
			map[string]string{
				`<tree_sitter/parser.h>`: `"parser.h"`,
				`"tree_sitter/parser.h"`: `"parser.h"`,
				`"tree_sitter/array.h"`:  `"../array.h"`,
				`<tree_sitter/array.h>`:  `"../array.h"`,
				`"tree_sitter/alloc.h"`:  `"../alloc.h"`,
				`<tree_sitter/alloc.h>`:  `"../alloc.h"`,
			},
		)
	}
}

// localPatchMarker is the phrase that opens the hand-maintained header block
// of a vendored grammar file carrying local modifications.
const localPatchMarker = "Locally patched"

// allowOverwritePatchedEnv lets a caller proceed past the patch guard once the
// patches have been re-applied by hand. Only the exact value "1" enables it:
// setting a flag to "0" or "false" to mean off is an ordinary shell habit, and
// treating those as "on" would silently disable the protection.
const allowOverwritePatchedEnv = "ALLOW_OVERWRITE_PATCHED"

func overwritePatchedAllowed() bool {
	return os.Getenv(allowOverwritePatchedEnv) == "1"
}

// isLocallyPatched reports whether a vendored file carries the hand-maintained
// header marking local divergence from upstream.
//
// Only the head of the file is read: the header block sits at the top, and
// generated parser tables run to tens of megabytes.
func isLocallyPatched(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false // no such file, nothing to protect
	}
	defer f.Close()

	head := make([]byte, 8192)
	n, _ := io.ReadFull(f, head)
	return bytes.Contains(head[:n], []byte(localPatchMarker))
}

// locallyPatchedFiles returns every file under a grammar's directory that
// carries the marker.
//
// The whole directory is walked rather than reconstructing each downloader's
// write targets: defaultGrammarDownload writes <lang>/<file>, downloadPhp also
// writes <lang>/tree_sitter/*, and downloadTypescript writes under
// <lang>/typescript/ and <lang>/tsx/. Walking covers all of them, and any
// future variant, without the path list drifting out of step.
func locallyPatchedFiles(langDir string) []string {
	var patched []string
	_ = filepath.WalkDir(langDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry is not something to protect
		}
		if isLocallyPatched(path) {
			patched = append(patched, path)
		}
		return nil
	})
	sort.Strings(patched)
	return patched
}

// patchGuardTripped records that the per-file backstop blocked a write, so the
// run can exit non-zero after it finishes.
//
// The backstop cannot exit on the spot: downloadFile runs inside the goroutines
// UpdateAll spawns, and writeGrammarsFile only runs after wg.Wait() returns, so
// terminating there would leave every grammar that did re-download recorded at
// its old revision. That is the same desync the skip in downloadGrammar exists
// to avoid, and a rarer trigger does not make it less damaging.
var patchGuardTripped atomic.Bool

// blockIfLocallyPatched is the last-resort per-file backstop, and reports
// whether the write must be skipped. downloadGrammar already skips a patched
// grammar before any write, so reaching this means a downloader wrote outside
// its own <lang>/ directory: a bug in that downloader rather than a routine
// condition, which is why it is loud and fails the run.
//
// It signals only through patchGuardTripped: downloadFile discards the result,
// so downloadGrammar still reports success and Update keeps the advanced pin.
// The run fails, but grammars.json is written first and records a revision the
// vendored sources do not match. Holding the pin back would mean threading an
// error return through every downloader, which is not worth it while the
// pre-scan covers every write target. If a downloader ever writes outside its
// own <lang>/ directory, widen the pre-scan rather than relying on this.
func blockIfLocallyPatched(ctx context.Context, toPath string) bool {
	if overwritePatchedAllowed() || !isLocallyPatched(toPath) {
		return false
	}

	patchGuardTripped.Store(true)
	getLogger(ctx).Error(
		"refusing to overwrite a locally patched file written outside the grammar directory scanned "+
			"before download; re-apply the patches described in its header against the new upstream "+
			"baseline, regenerate, and replace it by hand; set "+allowOverwritePatchedEnv+"=1 to proceed anyway",
		"path", toPath,
	)
	return true
}

func (s *UpdateService) downloadFile(ctx context.Context, url, toPath string, replaceMap map[string]string) {
	if blockIfLocallyPatched(ctx, toPath) {
		return
	}

	b := s.fetchFile(ctx, url)

	for old, new := range replaceMap {
		b = bytes.ReplaceAll(b, []byte(old), []byte(new))
	}

	err := os.WriteFile(toPath, b, 0644)
	if err != nil {
		logAndExit(getLogger(ctx), err.Error(), "path", toPath)
	}
}

func (s *UpdateService) fetchFile(ctx context.Context, url string) []byte {
	logger := getLogger(ctx).With("url", url)
	logger.Debug("fetching")

	resp, err := http.Get(url)
	if err != nil {
		logAndExit(logger, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logger.Error("incorrect response status code", "statusCode", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		logAndExit(logger, err.Error())
	}
	return b
}

func (s *UpdateService) writeGrammarsFile(ctx context.Context) {
	b, err := json.MarshalIndent(s.grammars, "", "  ")
	if err != nil {
		logAndExit(getLogger(ctx), fmt.Sprintf("marshaling grammars file error: %w", err))
	}
	err = os.WriteFile(grammarsJson, b, 0644)
	if err != nil {
		logAndExit(getLogger(ctx), err.Error(), "file", grammarsJson)
	}
}

func (s *UpdateService) downloadPhp(ctx context.Context, g *Grammar) {
	fileMapping := map[string]string{
		"parser.c":  "php/src/parser.c",
		"scanner.c": "php/src/scanner.c",
		"scanner.h": "common/scanner.h",
	}

	url := g.ContentURL()

	treeSitterFiles := []string{"parser.h", "array.h", "alloc.h"}

	for _, f := range treeSitterFiles {
		s.downloadFile(
			ctx,
			fmt.Sprintf("%s/%s/php/src/tree_sitter/%s", url, g.Revision, f),
			fmt.Sprintf("%s/tree_sitter/%s", g.Language, f),
			nil,
		)
	}

	for _, f := range g.Files {
		fp, ok := fileMapping[f]
		if !ok {
			logAndExit(getLogger(ctx), "mapping for file not found", "file", f)
		}

		s.downloadFile(
			ctx,
			fmt.Sprintf("%s/%s/%s", url, g.Revision, fp),
			fmt.Sprintf("%s/%s", g.Language, f),
			map[string]string{
				`<tree_sitter/parser.h>`:   `"parser.h"`,
				`"../../common/scanner.h"`: `"scanner.h"`,
			},
		)
	}
}

func (s *UpdateService) downloadDockerfile(ctx context.Context, g *Grammar) {
	fileMapping := map[string]string{
		"parser.h":  "src/tree_sitter/parser.h",
		"parser.c":  "src/parser.c",
		"scanner.c": "src/scanner.c",
	}

	url := g.ContentURL()
	for _, f := range g.Files {
		fp, ok := fileMapping[f]
		if !ok {
			logAndExit(getLogger(ctx), "mapping for file not found", "file", f)
		}

		s.downloadFile(
			ctx,
			fmt.Sprintf("%s/%s/%s", url, g.Revision, fp),
			fmt.Sprintf("%s/%s", g.Language, f),
			map[string]string{
				`"tree_sitter/parser.h"`: `"parser.h"`,
				`<tree_sitter/parser.h>`: `"parser.h"`,
			},
		)
	}
}

// ocaml is special since its folder structure is different from the other ones
func (s *UpdateService) downloadOcaml(ctx context.Context, g *Grammar) {
	fileMapping := map[string]string{
		"parser.c":  "grammars/ocaml/src/parser.c",
		"scanner.c": "grammars/ocaml/src/scanner.c",
		"scanner.h": "include/scanner.h",
		"alloc.h":   "include/tree_sitter/alloc.h",
		"parser.h":  "include/tree_sitter/parser.h",
	}

	url := g.ContentURL()
	for _, f := range g.Files {
		fp, ok := fileMapping[f]
		if !ok {
			logAndExit(getLogger(ctx), "mapping for file not found", "file", f)
		}

		s.downloadFile(
			ctx,
			fmt.Sprintf("%s/%s/%s", url, g.Revision, fp),
			fmt.Sprintf("%s/%s", g.Language, f),
			map[string]string{
				`"tree_sitter/alloc.h"`:        `"alloc.h"`,
				`"tree_sitter/parser.h"`:       `"parser.h"`,
				`<tree_sitter/parser.h>`:       `"parser.h"`,
				`"../../../include/scanner.h"`: `"scanner.h"`,
			},
		)
	}
}

// typescript is special as it contains 2 different grammars
func (s *UpdateService) downloadTypescript(ctx context.Context, g *Grammar) {
	url := g.ContentURL()

	langs := []string{"typescript", "tsx"}
	for _, lang := range langs {
		s.makeDir(ctx, fmt.Sprintf("%s/%s", g.Language, lang))

		s.downloadFile(
			ctx,
			fmt.Sprintf("%s/%s/common/scanner.h", url, g.Revision),
			fmt.Sprintf("%s/%s/scanner.h", g.Language, lang),
			map[string]string{
				`"tree_sitter/parser.h"`: `"parser.h"`,
				`<tree_sitter/parser.h>`: `"parser.h"`,
			},
		)
		s.downloadFile(
			ctx,
			fmt.Sprintf("%s/%s/%s/src/tree_sitter/parser.h", url, g.Revision, lang),
			fmt.Sprintf("%s/%s/parser.h", g.Language, lang),
			nil,
		)

		for _, f := range g.Files {
			s.downloadFile(
				ctx,
				fmt.Sprintf("%s/%s/%s/src/%s", url, g.Revision, lang, f),
				fmt.Sprintf("%s/%s/%s", g.Language, lang, f),
				map[string]string{
					`"tree_sitter/parser.h"`:   `"parser.h"`,
					`<tree_sitter/parser.h>`:   `"parser.h"`,
					`"../../common/scanner.h"`: `"scanner.h"`,
				},
			)
		}
	}
}

// markdown is special as it contains 2 different grammars
func (s *UpdateService) downloadMarkdown(ctx context.Context, g *Grammar) {
	url := g.ContentURL()

	langs := []string{"tree-sitter-markdown", "tree-sitter-markdown-inline"}
	for _, lang := range langs {
		s.makeDir(ctx, fmt.Sprintf("%s/%s", g.Language, lang))

		s.downloadFile(
			ctx,
			fmt.Sprintf("%s/%s/%s/src/tree_sitter/parser.h", url, g.Revision, lang),
			fmt.Sprintf("%s/%s/parser.h", g.Language, lang),
			nil,
		)

		for _, f := range g.Files {
			s.downloadFile(
				ctx,
				fmt.Sprintf("%s/%s/%s/src/%s", url, g.Revision, lang, f),
				fmt.Sprintf("%s/%s/%s", g.Language, lang, f),
				map[string]string{
					`"tree_sitter/parser.h"`: `"parser.h"`,
				},
			)
		}
	}
}

// for yaml grammar scanner.cc includes schema.generated.cc file
// it causes cgo to compile schema.generated.cc twice and throw duplicate symbols error
func (s *UpdateService) downloadYaml(ctx context.Context, g *Grammar) {
	s.defaultGrammarDownload(ctx, g)

	f1 := fmt.Sprintf("%s/schema.generated.cc", g.Language)
	f2 := fmt.Sprintf("%s/scanner.cc", g.Language)

	f1b, _ := os.ReadFile(f1)
	f2b, _ := os.ReadFile(f2)
	os.Remove(f1)

	// combine both files into one
	b := bytes.Join([][]byte{
		[]byte(`#include "parser.h"`),
		f1b,
		f2b,
	}, []byte("\n"))
	// remove include expression
	b = bytes.ReplaceAll(b, []byte(`#include "./schema.generated.cc"`), []byte(""))

	_ = os.WriteFile(fmt.Sprintf("%s/scanner.cc", g.Language), b, 0644)
}

// sql is special since its folder structure is different from the other ones
func (s *UpdateService) downloadSql(ctx context.Context, g *Grammar) {
	fileMapping := map[string]string{
		"parser.h":  "tree_sitter/parser.h",
		"scanner.c": "scanner.c",
		"parser.c":  "parser.c",
	}

	s.makeDir(ctx, fmt.Sprintf("%s/tree_sitter", g.Language))

	url := g.ContentURL()
	for _, f := range g.Files {
		fp, ok := fileMapping[f]
		if !ok {
			logAndExit(getLogger(ctx), "mapping for file not found", "file", f)
		}

		s.downloadFile(
			ctx,
			fmt.Sprintf("%s/%s/src/%s", url, g.Revision, fp),
			fmt.Sprintf("%s/%s", g.Language, fp),
			nil,
		)
	}
}

func logAndExit(logger *Logger, msg string, args ...interface{}) {
	logger.Error(msg, args...)
	os.Exit(1)
}

// Git

func fetchLastTag(repository string) (string, string) {
	cmd := exec.Command("git", "ls-remote", "--tags", "--sort", "-v:refname", repository, "v*")
	b, err := cmd.Output()
	if err != nil {
		logAndExit(defaultLogger, err.Error())
	}
	line := strings.SplitN(string(b), "\n", 2)[0]

	//handle situation when nothing is returned because tag doesn't start with `v`
	if len(line) < 1 {
		cmd := exec.Command("git", "ls-remote", "--tags", "--sort", "-v:refname", repository)
		b, err := cmd.Output()
		if err != nil {
			logAndExit(defaultLogger, err.Error())
		}
		line = strings.SplitN(string(b), "\n", 2)[0]
	}
	parts := strings.Split(line, "\t")

	tag := strings.TrimRight(strings.Split(parts[1], "/")[2], "^{}")
	rev := strings.Split(parts[0], "^")[0]

	return tag, rev
}

func fetchLastCommit(repository, branch string) string {
	cmd := exec.Command("git", "ls-remote", "--heads", repository, fmt.Sprint("refs/heads/", branch))
	b, err := cmd.Output()
	if err != nil {
		logAndExit(defaultLogger, err.Error())
	}

	return strings.Split(string(b), "\t")[0]
}

// Logging

type loggerCtxType string

const loggerCtxKey loggerCtxType = "loggerCtx"

func getLogger(ctx context.Context) *Logger {
	v := ctx.Value(loggerCtxKey)
	if v == nil {
		return defaultLogger
	}
	logger, ok := v.(*Logger)
	if !ok {
		return defaultLogger
	}
	return logger
}

// minimal structured logger to avoid external dependencies
// implements subset of golang.org/x/exp/slog interface
type Logger struct {
	level   Level
	records map[string]interface{}
}

func NewLogger() *Logger {
	return &Logger{LevelInfo, make(map[string]interface{})}
}

type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)

func (l *Logger) With(args ...interface{}) *Logger {
	records := make(map[string]interface{})
	for k, v := range l.records {
		records[k] = v
	}

	i := 0
	for i < len(args)-1 {
		records[args[i].(string)] = args[i+1]
		i += 2
	}

	return &Logger{l.level, records}
}

func (l *Logger) Error(msg string, args ...interface{}) {
	l.log(LevelError, msg, args...)
}

func (l *Logger) Warn(msg string, args ...interface{}) {
	l.log(LevelWarn, msg, args...)
}

func (l *Logger) Info(msg string, args ...interface{}) {
	l.log(LevelInfo, msg, args...)
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	l.log(LevelDebug, msg, args...)
}

func (l *Logger) log(level Level, msg string, args ...interface{}) {
	if level < l.level {
		return
	}

	printArgs := []interface{}{msg}
	for k, v := range l.records {
		printArgs = append(printArgs, fmt.Sprintf("%v=%v", k, v))
	}
	for k, v := range args {
		printArgs = append(printArgs, fmt.Sprintf("%v=%v", k, v))
	}

	fmt.Println(printArgs...)
}

// CLI

func flagsParse(fs *flag.FlagSet, args []string) {
	debug := fs.Bool("debug", false, "enable debug output")
	fs.Parse(args)

	if *debug {
		defaultLogger.level = LevelDebug
	}
}
