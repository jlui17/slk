package messages

var timestampFormat string

// SetTimestampFormat sets the clock format a linked message's card
// header uses: the config's timestamp_format, as message rows wear it.
func SetTimestampFormat(format string) { timestampFormat = format }

// TimestampFormat is the layout SetTimestampFormat set, the default
// "3:04 PM" until it is called.
func TimestampFormat() string {
	if timestampFormat == "" {
		return "3:04 PM"
	}
	return timestampFormat
}
