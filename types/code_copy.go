package types

type CodeCopyOp struct {
	CodeCopyDestOffset uint64 `json:"dest_offset,omitempty"`
	CodeCopyOffset     uint64 `json:"offset,omitempty"`
	CodeCopyLength     uint64 `json:"length,omitempty"`
}
