package wsutil

import (
	"bytes"
	"io"
	"testing"

	"github.com/gobwas/ws"
)

func TestReadMessageEOF(t *testing.T) {
	for _, test := range []struct {
		source   func() io.Reader
		messages []Message
		err      error
	}{
		{
			source: func() io.Reader { return eofReader },
			err:    io.EOF,
		},
		{
			source: func() io.Reader {
				// This case tests that ReadMessage still fails after
				// successfully reading header bytes frame via ws.ReadHeader()
				// and non-successfully read of the body.
				var buf bytes.Buffer
				f := ws.NewTextFrame([]byte("this part will be lost"))
				if err := ws.WriteHeader(&buf, f.Header); err != nil {
					t.Fatal(err)
				}
				return &buf
			},
			err: io.ErrUnexpectedEOF,
		},
		{
			source: func() io.Reader {
				// This case tests that ReadMessage not fail when reading
				// fragmented messages.
				var buf bytes.Buffer
				fs := []ws.Frame{
					ws.NewFrame(ws.OpText, false, []byte("fragment1")),
					ws.NewFrame(ws.OpContinuation, false, []byte(",")),
					ws.NewFrame(ws.OpContinuation, true, []byte("fragment2")),
				}
				for _, f := range fs {
					if err := ws.WriteFrame(&buf, f); err != nil {
						t.Fatal(err)
					}
				}
				return &buf
			},
			messages: []Message{
				{ws.OpText, []byte("fragment1,fragment2")},
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			ms, err := ReadMessage(test.source(), 0, nil)
			if err != test.err {
				t.Errorf("unexpected error: %v; want %v", err, test.err)
			}
			if n := len(ms); n != len(test.messages) {
				t.Fatalf("unexpected number of read messages: %d; want %d", n, 0)
			}
			for i, exp := range test.messages {
				act := ms[i]
				if act.OpCode != exp.OpCode {
					t.Errorf(
						"unexpected #%d message op code: %v; want %v",
						i, act.OpCode, exp.OpCode,
					)
				}
				if !bytes.Equal(act.Payload, exp.Payload) {
					t.Errorf(
						"unexpected #%d message payload: %q; want %q",
						i, string(act.Payload), string(exp.Payload),
					)
				}
			}
		})
	}
}

func TestReadMessageOversized(t *testing.T) {
	lengths := []int64{
		DefaultMaxFrameSize + 1,
		1 << 34, // 16 GiB
	}

	for _, length := range lengths {
		var buf bytes.Buffer
		hdr := ws.Header{
			Fin:    true,
			OpCode: ws.OpBinary,
			Length: length,
		}
		if err := ws.WriteHeader(&buf, hdr); err != nil {
			t.Fatalf("WriteHeader() unexpected error: %v", err)
		}

		ms, err := ReadMessage(&buf, 0, nil)
		if err != ErrFrameTooLarge {
			t.Fatalf("ReadMessage(len=%d) error = %v; want %v", length, err, ErrFrameTooLarge)
		}
		if len(ms) != 0 {
			t.Fatalf("ReadMessage(len=%d) returned %d messages; want 0", length, len(ms))
		}
	}
}

func TestReadDataOversized(t *testing.T) {
	var buf bytes.Buffer
	hdr := ws.Header{
		Fin:    true,
		OpCode: ws.OpBinary,
		Length: 1 << 34, // 16 GiB
	}
	if err := ws.WriteHeader(&buf, hdr); err != nil {
		t.Fatalf("WriteHeader() unexpected error: %v", err)
	}

	p, _, err := ReadData(&buf, 0)
	if err != ErrFrameTooLarge {
		t.Fatalf("ReadData() error = %v; want %v", err, ErrFrameTooLarge)
	}
	if p != nil {
		t.Fatalf("ReadData() payload = %v; want nil", p)
	}
}
