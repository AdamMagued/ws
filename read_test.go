package ws

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"testing"
)

func TestReadHeader(t *testing.T) {
	for i, test := range append([]RWTestCase{
		{
			Data: bits("0000 0000 0 1111111 10000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000"),
			//                              _______________________________________________________________________
			//                                                                 |
			//                                                            Length value
			Err: true,
		},
	}, RWTestCases...) {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			r := bytes.NewReader(test.Data)
			h, err := ReadHeader(r)
			if test.Err && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !test.Err && err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if test.Err {
				return
			}
			if !reflect.DeepEqual(h, test.Header) {
				t.Errorf("ReadHeader()\nread:\n\t%#v\nwant:\n\t%#v", h, test.Header)
			}
		})
	}
}

func TestReadFrameOversized(t *testing.T) {
	lengths := []int64{
		DefaultMaxFrameSize + 1,
		1 << 34, // 16 GiB
		0x7f00000000000000,
	}

	for _, length := range lengths {
		t.Run(fmt.Sprintf("len_%d", length), func(t *testing.T) {
			var buf bytes.Buffer
			hdr := Header{
				Fin:    true,
				OpCode: OpBinary,
				Length: length,
			}
			if err := WriteHeader(&buf, hdr); err != nil {
				t.Fatalf("WriteHeader() unexpected error: %v", err)
			}

			f, err := ReadFrame(&buf)
			if err != ErrFrameTooLarge {
				t.Fatalf("ReadFrame() error = %v; want %v", err, ErrFrameTooLarge)
			}
			if f.Payload != nil {
				t.Fatalf("ReadFrame() allocated payload of len %d; want nil", len(f.Payload))
			}
		})
	}
}

func TestReadFrameWithLimit(t *testing.T) {
	payload := []byte("hello websocket")
	raw := MustCompileFrame(NewTextFrame(payload))

	t.Run("within limit", func(t *testing.T) {
		f, err := ReadFrameWithLimit(bytes.NewReader(raw), int64(len(payload)))
		if err != nil {
			t.Fatalf("ReadFrameWithLimit() unexpected error: %v", err)
		}
		if !bytes.Equal(f.Payload, payload) {
			t.Fatalf("ReadFrameWithLimit() payload = %q; want %q", f.Payload, payload)
		}
	})

	t.Run("exceeds limit", func(t *testing.T) {
		f, err := ReadFrameWithLimit(bytes.NewReader(raw), int64(len(payload)-1))
		if err != ErrFrameTooLarge {
			t.Fatalf("ReadFrameWithLimit() error = %v; want %v", err, ErrFrameTooLarge)
		}
		if f.Payload != nil {
			t.Fatalf("ReadFrameWithLimit() payload = %v; want nil", f.Payload)
		}
	})

	t.Run("zero limit disabled", func(t *testing.T) {
		f, err := ReadFrameWithLimit(bytes.NewReader(raw), 0)
		if err != nil {
			t.Fatalf("ReadFrameWithLimit() unexpected error: %v", err)
		}
		if !bytes.Equal(f.Payload, payload) {
			t.Fatalf("ReadFrameWithLimit() payload = %q; want %q", f.Payload, payload)
		}
	})
}

func BenchmarkReadHeader(b *testing.B) {
	setup := func(header Header, n int) (rds []io.Reader) {
		bts := MustCompileFrame(Frame{Header: header})
		rds = make([]io.Reader, n)
		for i := 0; i < n; i++ {
			rds[i] = bytes.NewReader(bts)
		}

		return
	}

	for i, bench := range RWBenchCases {
		b.Run(fmt.Sprintf("%s#%d", bench.label, i), func(b *testing.B) {
			rds := setup(bench.header, b.N)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := ReadHeader(rds[i])
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("reused-buffer-%s#%d", bench.label, i), func(b *testing.B) {
			rds := setup(bench.header, b.N)
			bts := make([]byte, MaxHeaderSize)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := ReadHeaderBuffer(rds[i], bts)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
