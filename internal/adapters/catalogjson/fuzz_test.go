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
