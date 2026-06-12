package tui

import "testing"

func TestParseSlashCommandSupportsQuotedArgs(t *testing.T) {
	cmd, ok := parseSlashCommand(`/importbench "C:\Users\DELL\拆文 库\示例书" name=demo-book`)
	if !ok {
		t.Fatal("expected slash command")
	}
	if cmd.name != "importbench" {
		t.Fatalf("name = %q, want importbench", cmd.name)
	}
	if len(cmd.args) != 2 {
		t.Fatalf("args = %#v, want source and name", cmd.args)
	}
	if cmd.args[0] != `C:\Users\DELL\拆文 库\示例书` {
		t.Fatalf("source arg = %q", cmd.args[0])
	}
	if cmd.args[1] != "name=demo-book" {
		t.Fatalf("name arg = %q", cmd.args[1])
	}
}

func TestParseSlashCommandKeepsUnquotedBehavior(t *testing.T) {
	cmd, ok := parseSlashCommand(`/importbench ./bench name=demo-book`)
	if !ok {
		t.Fatal("expected slash command")
	}
	if cmd.name != "importbench" || len(cmd.args) != 2 || cmd.args[0] != "./bench" || cmd.args[1] != "name=demo-book" {
		t.Fatalf("unexpected command: %+v", cmd)
	}
}
