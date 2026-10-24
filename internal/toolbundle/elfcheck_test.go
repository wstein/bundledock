package toolbundle

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"strings"
	"testing"
)

type fixtureProg struct {
	typ   elf.ProgType
	flags elf.ProgFlag
}

// fixtureELF describes a minimal ELF64 image: header, program headers and an
// optional PT_DYNAMIC segment. No compiler is involved.
type fixtureELF struct {
	class   elf.Class
	data    elf.Data
	typ     elf.Type
	machine elf.Machine
	progs   []fixtureProg
	// dynamic lists the tags of the dynamic section; DT_NULL is appended.
	dynamic []elf.DynTag
}

func staticExec(machine elf.Machine) fixtureELF {
	return fixtureELF{class: elf.ELFCLASS64, data: elf.ELFDATA2LSB, typ: elf.ET_EXEC, machine: machine,
		progs: []fixtureProg{{elf.PT_LOAD, elf.PF_R | elf.PF_X}, {elf.PT_GNU_STACK, elf.PF_R | elf.PF_W}}}
}

func staticPIE(machine elf.Machine) fixtureELF {
	f := staticExec(machine)
	f.typ = elf.ET_DYN
	f.progs = append(f.progs, fixtureProg{elf.PT_DYNAMIC, elf.PF_R | elf.PF_W})
	f.dynamic = []elf.DynTag{elf.DT_FLAGS_1}
	return f
}

func (f fixtureELF) bytes() []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	header := make([]byte, 64)
	copy(header, "\x7fELF")
	header[4], header[5], header[6] = byte(f.class), byte(f.data), 1
	le.PutUint16(header[16:], uint16(f.typ))
	le.PutUint16(header[18:], uint16(f.machine))
	le.PutUint32(header[20:], 1)
	le.PutUint64(header[32:], 64) // e_phoff
	le.PutUint16(header[52:], 64) // e_ehsize
	le.PutUint16(header[54:], 56) // e_phentsize
	le.PutUint16(header[56:], uint16(len(f.progs)))
	b.Write(header)
	dynamicOffset := uint64(64 + 56*len(f.progs))
	dynamicSize := uint64(16 * (len(f.dynamic) + 1))
	for _, p := range f.progs {
		ph := make([]byte, 56)
		le.PutUint32(ph[0:], uint32(p.typ))
		le.PutUint32(ph[4:], uint32(p.flags))
		if p.typ == elf.PT_DYNAMIC {
			le.PutUint64(ph[8:], dynamicOffset)
			le.PutUint64(ph[32:], dynamicSize)
			le.PutUint64(ph[40:], dynamicSize)
		}
		b.Write(ph)
	}
	for _, tag := range append(append([]elf.DynTag{}, f.dynamic...), elf.DT_NULL) {
		entry := make([]byte, 16)
		le.PutUint64(entry[0:], uint64(tag))
		b.Write(entry)
	}
	return b.Bytes()
}

func TestCheckStaticELFAcceptsDeclaredForm(t *testing.T) {
	for name, tc := range map[string]struct {
		file fixtureELF
		arch string
		form string
	}{
		"exec amd64":       {staticExec(elf.EM_X86_64), "amd64", "exec"},
		"exec arm64":       {staticExec(elf.EM_AARCH64), "arm64", "exec"},
		"static-pie amd64": {staticPIE(elf.EM_X86_64), "amd64", "static-pie"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := CheckStaticELF(bytes.NewReader(tc.file.bytes()), tc.arch, tc.form); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCheckStaticELFRejectsDynamicAndWrongOutput(t *testing.T) {
	withProg := func(f fixtureELF, p fixtureProg) fixtureELF { f.progs = append(f.progs, p); return f }
	dynamicExec := func(tags ...elf.DynTag) fixtureELF {
		f := staticExec(elf.EM_X86_64)
		f.progs = append(f.progs, fixtureProg{elf.PT_DYNAMIC, elf.PF_R | elf.PF_W})
		f.dynamic = tags
		return f
	}
	for name, tc := range map[string]struct {
		file fixtureELF
		arch string
		form string
		want string
	}{
		"interpreter":     {withProg(staticExec(elf.EM_X86_64), fixtureProg{elf.PT_INTERP, elf.PF_R}), "amd64", "exec", "PT_INTERP"},
		"dynamic in exec": {dynamicExec(elf.DT_FLAGS_1), "amd64", "exec", "PT_DYNAMIC"},
		"needed library":  {func() fixtureELF { f := staticPIE(elf.EM_X86_64); f.dynamic = []elf.DynTag{elf.DT_NEEDED}; return f }(), "amd64", "static-pie", "DT_NEEDED"},
		"rpath":           {func() fixtureELF { f := staticPIE(elf.EM_X86_64); f.dynamic = []elf.DynTag{elf.DT_RPATH}; return f }(), "amd64", "static-pie", "DT_RPATH"},
		"runpath": {func() fixtureELF {
			f := staticPIE(elf.EM_X86_64)
			f.dynamic = []elf.DynTag{elf.DT_FLAGS_1, elf.DT_RUNPATH}
			return f
		}(), "amd64", "static-pie", "DT_RUNPATH"},
		"static-pie without dynamic section": {func() fixtureELF { f := staticExec(elf.EM_X86_64); f.typ = elf.ET_DYN; return f }(), "amd64", "static-pie", "PT_DYNAMIC"},
		"exec declared, pie built":           {staticPIE(elf.EM_X86_64), "amd64", "exec", "ET_EXEC"},
		"pie declared, exec built":           {staticExec(elf.EM_X86_64), "amd64", "static-pie", "ET_DYN"},
		"executable stack":                   {func() fixtureELF { f := staticExec(elf.EM_X86_64); f.progs[1].flags |= elf.PF_X; return f }(), "amd64", "exec", "stack"},
		"no stack header":                    {func() fixtureELF { f := staticExec(elf.EM_X86_64); f.progs = f.progs[:1]; return f }(), "amd64", "exec", "PT_GNU_STACK"},
		"no loadable segment":                {func() fixtureELF { f := staticExec(elf.EM_X86_64); f.progs = f.progs[1:]; return f }(), "amd64", "exec", "PT_LOAD"},
		"wrong machine":                      {staticExec(elf.EM_AARCH64), "amd64", "exec", "machine"},
		"wrong machine other way":            {staticExec(elf.EM_X86_64), "arm64", "exec", "machine"},
		"32-bit class":                       {func() fixtureELF { f := staticExec(elf.EM_X86_64); f.class = elf.ELFCLASS32; return f }(), "amd64", "exec", "ELFCLASS64"},
		"big endian":                         {func() fixtureELF { f := staticExec(elf.EM_X86_64); f.data = elf.ELFDATA2MSB; return f }(), "amd64", "exec", "little"},
		"unknown architecture":               {staticExec(elf.EM_X86_64), "riscv64", "exec", "architecture"},
		"unknown form":                       {staticExec(elf.EM_X86_64), "amd64", "pie", "form"},
	} {
		t.Run(name, func(t *testing.T) {
			err := CheckStaticELF(bytes.NewReader(tc.file.bytes()), tc.arch, tc.form)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v does not mention %q", err, tc.want)
			}
		})
	}
}

func TestCheckStaticELFRejectsNonELFAndTruncation(t *testing.T) {
	good := staticPIE(elf.EM_X86_64).bytes()
	for name, data := range map[string][]byte{
		"empty":             nil,
		"script":            []byte("#!/bin/sh\necho hi\n"),
		"truncated header":  good[:20],
		"truncated phdrs":   good[:100],
		"truncated dynamic": good[:len(good)-8],
	} {
		t.Run(name, func(t *testing.T) {
			if err := CheckStaticELF(bytes.NewReader(data), "amd64", "static-pie"); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
