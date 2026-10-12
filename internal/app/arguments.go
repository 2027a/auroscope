package app

import (
	"fmt"
	"strings"
)

// paruArguments separates operands from targets without interpreting their values.
// Arity follows pinned Paru 9ac3578, src/command_line.rs and src/args.rs. Paru
// remains responsible for option semantics and dependency resolution.
type paruArguments struct {
	options []string // original spelling and operands, for the final handoff
	flags   []string // option names only, for dispatch (never operand text)
	targets []string
}

func parseParuArguments(args []string) (paruArguments, error) {
	var parsed paruArguments
	end := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if end || arg == "-" || !strings.HasPrefix(arg, "-") {
			parsed.targets = append(parsed.targets, arg)
			continue
		}
		if arg == "--" {
			end = true
			continue
		}
		parsed.options = append(parsed.options, arg)
		needsNext := false
		if strings.HasPrefix(arg, "--") {
			name, _, attached := strings.Cut(arg, "=")
			arity := paruLongOptionArity(name)
			if arity < 0 {
				return paruArguments{}, fmt.Errorf("unsupported Paru option %q: cannot distinguish its value from a target", name)
			}
			if attached && arity == 0 {
				return paruArguments{}, fmt.Errorf("Paru option %q does not take a value", name)
			}
			needsNext = arity == 1 && !attached
			parsed.flags = append(parsed.flags, name)
		} else {
			// Only -b and -r consume a short-option operand. The rest of a
			// cluster is their attached value, not more option letters.
			for j := 1; j < len(arg); j++ {
				flag := arg[j]
				if !strings.ContainsRune("DFQRSTUBPGLCVhabrcdefgiklmnopqstuvwxy", rune(flag)) {
					return paruArguments{}, fmt.Errorf("unsupported Paru short option -%c", flag)
				}
				if flag == 'b' || flag == 'r' {
					needsNext = j == len(arg)-1
					parsed.flags = append(parsed.flags, arg[:j+1])
					break
				}
				if j == len(arg)-1 {
					parsed.flags = append(parsed.flags, arg)
				}
			}
		}
		if needsNext {
			i++
			if i == len(args) {
				return paruArguments{}, fmt.Errorf("Paru option %q expects a value", arg)
			}
			parsed.options = append(parsed.options, args[i])
		}
	}
	return parsed, nil
}

// 0: no value; 1: required; 2: optional, accepted only with '='. Unknown
// options fail closed rather than guessing whether the next word is a target.
func paruLongOptionArity(name string) int {
	switch name {
	case "--aururl", "--aurrpcurl", "--makepkg", "--pacman", "--pacman-conf",
		"--git", "--gpg", "--sudo", "--pkgctl", "--fm", "--bat", "--makepkgconf",
		"--mflags", "--gitflags", "--gpgflags", "--sudoflags", "--batflags", "--fmflags",
		"--chrootflags", "--chrootpkgs", "--rootchrootpkgs", "--completioninterval",
		"--sortby", "--searchby", "--limit", "--develsuffixes", "--builddir", "--clonedir",
		"--develfile", "--dbpath", "--root", "--ask", "--cachedir", "--arch", "--color",
		"--config", "--gpgdir", "--hookdir", "--logfile", "--sysroot", "--ignore",
		"--ignoregroup", "--ignoredevel", "--assume-installed", "--print-format",
		"--overwrite", "--mode":
		return 1
	case "--removemake", "--redownload", "--rebuild", "--sudoloop", "--localrepo",
		"--chroot", "--provides", "--sign", "--signdb":
		return 2
	case "--help", "--version", "--sync", "--database", "--files", "--query",
		"--remove", "--deptest", "--upgrade", "--build", "--show", "--getpkgbuild",
		"--repoctl", "--chrootctl", "--aur", "--repo", "--pkgbuilds", "--interactive",
		"--skipreview", "--review", "--gendb", "--nocheck", "--devel", "--nodevel",
		"--noprovides", "--pgpfetch", "--nopgpfetch", "--useask", "--nouseask",
		"--savechanges", "--nosavechanges", "--combinedupgrade", "--nocombinedupgrade",
		"--batchinstall", "--nobatchinstall", "--nosudoloop", "--noconfirm", "--confirm",
		"--installdebug", "--noinstalldebug", "--news", "--stats", "--order",
		"--upgrademenu", "--noupgrademenu", "--noremovemake", "--cleanafter",
		"--nocleanafter", "--noredownload", "--norebuild", "--topdown", "--bottomup",
		"--complete", "--install", "--delete", "--noinstall", "--newsonupgrade",
		"--nonewsonupgrade", "--comments", "--ssh", "--failfast", "--nofailfast",
		"--keepsrc", "--nokeepsrc", "--nolocalrepo", "--nochroot", "--nokeeprepocache",
		"--keeprepocache", "--nosign", "--nosigndb", "--verbose", "--debug",
		"--disable-download-timeout", "--nodeps", "--dbonly", "--noprogressbar",
		"--noscriptlet", "--print", "--asdeps", "--asdep", "--asexplicit", "--asexp",
		"--needed", "--force", "--changelog", "--deps", "--explicit", "--groups",
		"--info", "--check", "--list", "--foreign", "--native", "--owns", "--file",
		"--quiet", "--search", "--unrequired", "--upgrades", "--cascade", "--nosave",
		"--recursive", "--unneeded", "--clean", "--optional", "--sysupgrade",
		"--downloadonly", "--refresh", "--regex", "--machinereadable":
		return 0
	default:
		return -1
	}
}

func (parsed paruArguments) classificationArgs() []string {
	args := append([]string(nil), parsed.flags...)
	if len(parsed.targets) > 0 {
		// Only target presence matters to dispatch. A target named --repo or
		// -Sup after '--' must never be mistaken for an option.
		args = append(args, "target")
	}
	return args
}

// Paru treats builddir and clonedir as aliases. For audited transactions these
// select the operation root, never a shared worktree. Only their own operands
// are rewritten; another option's value is opaque even if it resembles a flag.
func operationCloneOptions(options []string, root string, replace bool) (string, []string) {
	options = append([]string(nil), options...)
	for i := 0; i < len(options); i++ {
		arg := options[i]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "--") {
			name, value, attached := strings.Cut(arg, "=")
			if name == "--clonedir" || name == "--builddir" {
				if !attached {
					value = options[i+1]
				}
				if replace {
					if attached {
						options[i] = name + "=" + root
					} else {
						options[i+1] = root
					}
				} else {
					root = value
				}
			}
			if !attached && paruLongOptionArity(name) == 1 {
				i++
			}
		} else {
			for j := 1; j < len(arg); j++ {
				if arg[j] == 'b' || arg[j] == 'r' {
					if j == len(arg)-1 {
						i++
					}
					break
				}
			}
		}
	}
	return root, options
}
