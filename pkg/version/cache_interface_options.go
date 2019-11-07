package version

type GetOptions struct{}

type GetOption func(*GetOptions)

func NewGetOptions(setters ...GetOption) *GetOptions {
	opts := &GetOptions{}

	for _, setter := range setters {
		setter(opts)
	}

	return opts
}
