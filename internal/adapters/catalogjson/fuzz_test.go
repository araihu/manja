package catalogjson

import "testing"

func FuzzDecodeCatalog(f *testing.F) {
	seed, err := EncodeCatalog(codecCatalog())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		value, err := DecodeCatalog(data)
		if err != nil {
			return
		}
		encoded, err := EncodeCatalog(value)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != string(data) {
			t.Fatal("accepted catalog bytes were not canonical")
		}
	})
}

func FuzzDecodeDetailShard(f *testing.F) {
	seed, err := EncodeDetailShard(codecDetailShard())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		value, err := DecodeDetailShard(data)
		if err != nil {
			return
		}
		encoded, err := EncodeDetailShard(value)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != string(data) {
			t.Fatal("accepted detail bytes were not canonical")
		}
	})
}

func FuzzDecodeSearchDirectory(f *testing.F) {
	seed, err := EncodeSearchDirectory(codecSearchDirectory())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		value, err := DecodeSearchDirectory(data)
		if err != nil {
			return
		}
		encoded, err := EncodeSearchDirectory(value)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != string(data) {
			t.Fatal("accepted search directory bytes were not canonical")
		}
	})
}

func FuzzDecodeSearchSegments(f *testing.F) {
	exact, err := EncodeSearchExactSegment(codecSearchExactSegment())
	if err != nil {
		f.Fatal(err)
	}
	posting, err := EncodeSearchPostingSegment(codecSearchPostingSegment())
	if err != nil {
		f.Fatal(err)
	}
	record, err := EncodeSearchRecordSegment(codecSearchRecordSegment())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(uint8(0), exact)
	f.Add(uint8(1), posting)
	f.Add(uint8(2), record)
	f.Fuzz(func(t *testing.T, kind uint8, data []byte) {
		switch kind % 3 {
		case 0:
			value, err := DecodeSearchExactSegment(data)
			if err != nil {
				return
			}
			encoded, err := EncodeSearchExactSegment(value)
			if err != nil || string(encoded) != string(data) {
				t.Fatalf("accepted exact segment was not canonical: %v", err)
			}
		case 1:
			value, err := DecodeSearchPostingSegment(data)
			if err != nil {
				return
			}
			encoded, err := EncodeSearchPostingSegment(value)
			if err != nil || string(encoded) != string(data) {
				t.Fatalf("accepted posting segment was not canonical: %v", err)
			}
		case 2:
			value, err := DecodeSearchRecordSegment(data)
			if err != nil {
				return
			}
			encoded, err := EncodeSearchRecordSegment(value)
			if err != nil || string(encoded) != string(data) {
				t.Fatalf("accepted record segment was not canonical: %v", err)
			}
		}
	})
}
