package source

// MediaTypeValue represents the media entity type discriminator.
// Used as the value for the MediaType predicate to distinguish image, video,
// keyframe, audio, and opaque binary entities stored in the same graph.
type MediaTypeValue string

const (
	// MediaTypeImage identifies a static image entity.
	MediaTypeImage MediaTypeValue = "image"

	// MediaTypeVideo identifies a video media entity.
	MediaTypeVideo MediaTypeValue = "video"

	// MediaTypeKeyframe identifies a keyframe extracted from a video.
	MediaTypeKeyframe MediaTypeValue = "keyframe"

	// MediaTypeAudio identifies an audio media entity.
	MediaTypeAudio MediaTypeValue = "audio"

	// MediaTypeBinary identifies an opaque binary artifact.
	MediaTypeBinary MediaTypeValue = "binary"
)
