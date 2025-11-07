package protoutil

import (
	"io"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// WriteDelimitedTo writes a length-delimited protobuf message to a writer
func WriteDelimitedTo(msg proto.Message, w io.Writer) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	// Write the size as a varint
	sizeBytes := make([]byte, protowire.SizeVarint(uint64(len(data))))
	protowire.AppendVarint(sizeBytes[:0], uint64(len(data)))

	// Write size followed by message data
	if _, err := w.Write(sizeBytes); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}

	return nil
}

// ReadDelimitedFrom reads a length-delimited protobuf message from a reader
func ReadDelimitedFrom(msg proto.Message, r io.Reader) error {
	// Read the size varint
	var sizeBuf [10]byte // Maximum size of a varint
	var n int
	var err error

	// Read first byte to determine varint length
	if n, err = r.Read(sizeBuf[:1]); err != nil {
		if err == io.EOF {
			return io.EOF
		}
		return err
	}

	size, bytesRead := protowire.ConsumeVarint(sizeBuf[:n])
	if bytesRead < 0 {
		// Need more bytes for the varint
		var i int
		for i = 1; i < len(sizeBuf) && bytesRead < 0; i++ {
			if _, err = r.Read(sizeBuf[i : i+1]); err != nil {
				return err
			}
			size, bytesRead = protowire.ConsumeVarint(sizeBuf[:i+1])
		}
		if bytesRead < 0 {
			return io.ErrUnexpectedEOF
		}
	}

	// Read the message data
	data := make([]byte, size)
	if _, err = io.ReadFull(r, data); err != nil {
		return err
	}

	// Unmarshal the message
	return proto.Unmarshal(data, msg)
}
