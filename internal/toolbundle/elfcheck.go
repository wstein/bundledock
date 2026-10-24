package toolbundle

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
)

var elfMachines = map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}

// CheckStaticELF proves that a source-build output is a self-contained ELF of the
// target architecture in the declared output form ("exec" for a static non-PIE
// ET_EXEC, "static-pie" for a static ET_DYN). It reads the program headers, not
// the section headers, so a stripped or section-less file cannot hide a loader
// or a library dependency. It does not prove that the program runs.
func CheckStaticELF(r io.ReaderAt, arch, form string) error {
	machine, ok := elfMachines[arch]
	if !ok {
		return fmt.Errorf("unsupported architecture %q", arch)
	}
	if form != "exec" && form != "static-pie" {
		return fmt.Errorf("unsupported output form %q", form)
	}
	// Class and byte order are read from the identification bytes first: debug/elf
	// would otherwise report a mis-parsed 32-bit or big-endian file as a format error.
	ident := make([]byte, elf.EI_NIDENT)
	if _, err := r.ReadAt(ident, 0); err != nil || string(ident[:4]) != elf.ELFMAG {
		return fmt.Errorf("not an ELF file")
	}
	if elf.Class(ident[elf.EI_CLASS]) != elf.ELFCLASS64 {
		return fmt.Errorf("ELF class %v, want ELFCLASS64", elf.Class(ident[elf.EI_CLASS]))
	}
	if elf.Data(ident[elf.EI_DATA]) != elf.ELFDATA2LSB {
		return fmt.Errorf("ELF is not little endian")
	}
	f, err := elf.NewFile(r)
	if err != nil {
		return fmt.Errorf("invalid ELF file: %w", err)
	}
	if f.Machine != machine {
		return fmt.Errorf("ELF machine %v does not match %s", f.Machine, arch)
	}
	wantType := elf.ET_EXEC
	if form == "static-pie" {
		wantType = elf.ET_DYN
	}
	if f.Type != wantType {
		return fmt.Errorf("ELF type %v, want %v for output form %s", f.Type, wantType, form)
	}
	var load, stack, dynamic *elf.Prog
	for _, p := range f.Progs {
		switch p.Type {
		case elf.PT_INTERP:
			return fmt.Errorf("ELF requests a dynamic loader (PT_INTERP)")
		case elf.PT_LOAD:
			load = p
		case elf.PT_GNU_STACK:
			stack = p
		case elf.PT_DYNAMIC:
			dynamic = p
		}
	}
	if load == nil {
		return fmt.Errorf("ELF has no PT_LOAD segment")
	}
	if stack == nil {
		return fmt.Errorf("ELF has no PT_GNU_STACK header")
	}
	if stack.Flags&elf.PF_X != 0 {
		return fmt.Errorf("ELF requests an executable stack")
	}
	if form == "exec" {
		if dynamic != nil {
			return fmt.Errorf("static executable has a PT_DYNAMIC segment")
		}
		return nil
	}
	if dynamic == nil {
		return fmt.Errorf("static-pie has no PT_DYNAMIC segment")
	}
	return checkDynamic(dynamic)
}

// checkDynamic rejects library and search-path entries in the dynamic segment.
func checkDynamic(p *elf.Prog) error {
	if p.Filesz > 1<<20 || p.Filesz%16 != 0 {
		return fmt.Errorf("invalid PT_DYNAMIC size %d", p.Filesz)
	}
	data := make([]byte, p.Filesz)
	if _, err := p.ReadAt(data, 0); err != nil {
		return fmt.Errorf("read PT_DYNAMIC: %w", err)
	}
	for i := 0; i+16 <= len(data); i += 16 {
		switch elf.DynTag(binary.LittleEndian.Uint64(data[i:])) {
		case elf.DT_NULL:
			return nil
		case elf.DT_NEEDED:
			return fmt.Errorf("ELF needs a shared library (DT_NEEDED)")
		case elf.DT_RPATH:
			return fmt.Errorf("ELF sets DT_RPATH")
		case elf.DT_RUNPATH:
			return fmt.Errorf("ELF sets DT_RUNPATH")
		}
	}
	return fmt.Errorf("PT_DYNAMIC is not terminated by DT_NULL")
}
