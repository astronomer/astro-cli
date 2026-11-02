package scaffold

// LinkNotePrefix starts each Left-to-do note naming a Deployment link a 1.x
// .astro/config.yaml listed and the conversion did not make. The rest of the
// note is the astro link add command for it. One note per link, so a json
// consumer gets each command on its own; the CLI's text output groups them
// under one line by this prefix.
const LinkNotePrefix = config1xRelPath + ": link the Deployment it lists with: "
