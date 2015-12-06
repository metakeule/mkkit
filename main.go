package main

import (
	"fmt"
	"github.com/metakeule/supergollider/note"
	"gopkg.in/metakeule/config.v1"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	args = config.MustNew("mkkit", "0.1", "generate sampson kits from a dir of sound files in wav format")

	dirArg = args.NewString("dir",
		"directory where the wav files reside",
		config.Default("."), config.Shortflag('d'))

	nameArg = args.NewString("name",
		"name of the sample groups",
		config.Default("mkkitgen"), config.Shortflag('n'))

	keysArg = args.NewInt32("keys",
		"number of keys per sample",
		config.Default(int32(0)), config.Shortflag('k'))

	startArg = args.NewInt32("start",
		"start key, minimum value is 12",
		config.Default(int32(int(note.C1))))

	relativeArg = args.NewBool("relative",
		"write relative paths of sample files to kit (instead of absolute paths which is the default)",
		config.Default(false))

	relArg = args.NewFloat32("rel",
		"release time of sample, if < 0: infinite (note off events are ignored and sample is always played until its end)",
		config.Default(float32(0.1)))

	fixedPitchArg = args.NewBool("fixed",
		"play all samples at fixed pitch",
		config.Default(false))

	refKeyArg = args.NewString("ref",
		"reference key, valid values are 'lo' and 'hi'. If not set a key in the middle is taken",
		config.Shortflag('e'))

	groupArg = args.NewInt32("group",
		"group property; 0 disables grouping and -1*n creates a new group every n samples",
		config.Default(int32(0)), config.Shortflag('g'))

	panArg = args.NewFloat32("pan",
		"panning value must be between -1.0 and 1.0",
		config.Default(float32(0.0)),
	)

	matchArg = args.NewString("match",
		"only use sound files matching the given regular expression (posix)",
		config.Shortflag('m'))
)

func main() {
	var (
		// define the variables here that are shared along the steps
		// most variables should only by defined by the type here
		// and are assigned inside the steps
		err error
		kit Kit
	)

steps:
	for jump := 1; err == nil; jump++ {
		switch jump - 1 {
		default:
			break steps
		// count a number up for each following step
		case 0:
			err = args.Run()
		case 1:
			kit.Name = nameArg.Get()
			kit.FixedPitch = fixedPitchArg.Get()
			kit.Keys = int(keysArg.Get())
			kit.StartKey = int(startArg.Get())
			kit.RelativePaths = relativeArg.Get()
			if groupArg.Get() != 0 {
				kit.Group = int(groupArg.Get())
			}
			kit.Dir, err = consolidateDir(dirArg.Get())
		case 2:
			if refKeyArg.IsSet() {
				ref := strings.ToLower(refKeyArg.Get())
				switch ref {
				case "lo", "hi":
					kit.RefKey = ref
				default:
					err = fmt.Errorf("invalid value %#v for --ref, allowed are only 'lo' and 'hi'", refKeyArg.Get())
				}
			}
		case 3:
			kit.Rel = relArg.Get()
			kit.Pan = panArg.Get()
			if kit.Pan < -1.0 || kit.Pan > 1.0 {
				err = fmt.Errorf("invalid value %#v for --pan, allowed are only values between -1.0 and 1.0", kit.Pan)
			}
		case 4:
			if matchArg.IsSet() {
				kit.Match, err = regexp.CompilePOSIX(matchArg.Get())
			}
		case 5:
			err = kit.ScanDir()
		case 6:
			fmt.Fprintf(os.Stdout, kit.String())
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
	}
}

type Sample struct {
	File       string
	FixedPitch bool
	KeyStart   note.Note
	KeyEnd     note.Note
	RefKey     note.Note
	Group      int
	Pan        float32
	Rel        float32
}

func (s Sample) String() (str string) {
	str = fmt.Sprintf("file %s\nkeyrange %s %s\n", s.File, noteName(s.KeyStart), noteName(s.KeyEnd))
	if s.RefKey != 0 {
		str += fmt.Sprintf("refkey %s\n", noteName(s.RefKey))
	}
	if s.FixedPitch {
		str += "fixedpitch\n"
	}
	if s.Group > 0 {
		str += fmt.Sprintf("group %d\n", s.Group)
	}

	if s.Pan != 0.0 {
		str += fmt.Sprintf("pan %0.1f\n", s.Pan)
	}

	if s.Rel >= 0.0 {
		str += fmt.Sprintf("rel %0.1f\n", s.Rel)
	}

	str += "--\n\n"
	return
}

type Kit struct {
	Name          string
	Dir           string
	FixedPitch    bool
	Samples       []Sample
	currentKey    note.Note
	StartKey      int // midinumber of key
	Keys          int // number of keys per sample
	RelativePaths bool
	RefKey        string
	Group         int
	Pan           float32
	Match         *regexp.Regexp
	Rel           float32
	currentGroup  int
}

func (k *Kit) ScanSample(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if path == k.Dir {
		return nil
	}

	var name = filepath.Base(path)

	if info.IsDir() {
		return filepath.SkipDir
	}

	if strings.ToLower(filepath.Ext(name)) == ".wav" {
		if k.Match != nil && !k.Match.MatchString(name) {
			return nil
		}
		k.currentKey++
		var s = Sample{File: path, FixedPitch: k.FixedPitch, KeyStart: k.currentKey}

		if k.RelativePaths {
			s.File = name
		}
		for i := 0; i < k.Keys-1; i++ {
			k.currentKey++
		}
		s.KeyEnd = k.currentKey

		if !k.FixedPitch {
			switch k.RefKey {
			case "lo":
				s.RefKey = s.KeyStart
			case "hi":
				s.RefKey = s.KeyEnd
			default:
				if s.KeyStart < s.KeyEnd {
					diff := s.KeyEnd - s.KeyStart
					s.RefKey = s.KeyStart + note.Note(float64(int(diff/2)))
				}
			}
		}

		if k.Group < 0 && (len(k.Samples))%(k.Group*-1) == 0 {
			k.currentGroup++
		}

		if k.currentGroup > 0 {
			s.Group = k.currentGroup
		}

		if k.Pan != 0.0 {
			s.Pan = k.Pan
		}

		s.Rel = k.Rel

		k.Samples = append(k.Samples, s)

	}

	return nil
}

func (k *Kit) ScanDir() error {
	if k.Group >= 0 {
		k.currentGroup = k.Group
	}
	start := k.StartKey - 1
	if start < 11 {
		start = 11
	}
	k.currentKey = note.Note(start)
	return filepath.Walk(k.Dir, k.ScanSample)
}

func (k *Kit) String() (s string) {
	if len(k.Samples) == 0 {
		return ""
	}
	s = fmt.Sprintf(";;; %s\n\n", k.Name)
	for _, sample := range k.Samples {
		s += sample.String()
	}
	return
}

func noteName(n note.Note) string {
	return strings.ToLower(strings.Replace(n.String(), "is", "#", -1))
}

func consolidateDir(dir string) (d string, err error) {
	d = dir
	if d == "." {
		d, err = os.Getwd()
	}

	if err != nil {
		return
	}

	d, err = filepath.Abs(d)
	return
}
