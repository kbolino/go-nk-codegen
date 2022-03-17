package main

const (
	// AttrNoStrLen applies to functions and indicates that they have string
	// parameter(s) without corresponding length parameter(s).
	AttrNoStrLen = "nostrlen"
	// AttrUnsafePtr applies to functions and indicates that they take pointer
	// parameters which must be cast through unsafe.Pointer. For example, Go
	// will not allow *uintptr to be cast to *C.size_t even though they are
	// equivalent in size. This is not necessary for struct types, since it
	// is assumed they can't be cast directly anyway.
	AttrUnsafePtr = "unsafeptr"
	// AttrUntyped applies to enums and indicates that their constants should
	// be untyped.
	AttrUntyped = "untyped"
)
