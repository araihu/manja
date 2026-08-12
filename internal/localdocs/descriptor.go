package localdocs

// DescriptorV1 carries the immutable, non-secret inputs required to admit a
// public documentation projection for local enhancement.
type DescriptorV1 struct {
	SchemaVersion         uint32 `json:"schemaVersion"`
	PublicationKey        string `json:"publicationKey"`
	PublicationBase       string `json:"publicationBase"`
	SnapshotID            string `json:"snapshotId"`
	RevisionID            string `json:"revisionId"`
	ProjectionFormat      string `json:"projectionFormat"`
	ProjectionDigest      string `json:"projectionDigest"`
	ProjectionManifestURL string `json:"projectionManifestUrl"`
	ProjectionDataBase    string `json:"projectionDataBase"`
}
