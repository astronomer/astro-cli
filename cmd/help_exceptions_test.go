package cmd

// helpExceptions lists, per rule in helpRules, the commands that failed it
// when the rule was written. The lists only shrink: fix a command's help and
// delete its line, and TestHelpExceptionsOnlyShrink fails on a line left
// behind. Nothing is added here — a new command meets the rules, and an
// existing one that gains a violation is fixed rather than excused.
//
// Every rule's list is empty today. The map stays, with
// TestHelpExceptionsOnlyShrink, for a rule written later that some commands
// fail on the day it lands.
var helpExceptions = map[string][]string{}
