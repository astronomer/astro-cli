package input

import "sync"

// A picker shared by several commands cannot know which option answers it: a
// user is an email argument to `astro workspace user remove` and a flag to
// `astro organization team user remove`. The command knows, and says so before
// it asks (HintAnswer); a refusal about that question, for any reason, then
// names it. The hints are process-level for the run, like the guard, and are
// cleared when the run ends (cliout.Execute).
var hints struct {
	sync.Mutex
	byAbout map[string]string
}

// HintAnswer says that, for a question about about (the About option's text),
// answeredBy is what answers it in this run.
func HintAnswer(about, answeredBy string) {
	hints.Lock()
	defer hints.Unlock()
	if hints.byAbout == nil {
		hints.byAbout = map[string]string{}
	}
	hints.byAbout[about] = answeredBy
}

// ClearHints forgets every hint.
func ClearHints() {
	hints.Lock()
	defer hints.Unlock()
	hints.byAbout = nil
}

// ApplyHint names the hinted answer in e when the asker named none, and returns
// e.
func ApplyHint(e *RequiredError) *RequiredError {
	if e.Flag != "" || e.About == "" {
		return e
	}
	hints.Lock()
	defer hints.Unlock()
	if answer, ok := hints.byAbout[e.About]; ok {
		e.Flag = answer
	}
	return e
}
