package channels_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
	_ "github.com/vasic-digital/herald/commons_messaging/channels/tgram" // blank import forces tgram's init() (TestTgramRegisteredViaInit)
)

// fakeChannel is a trivial channels.Channel implementation shared by the
// registry tests. It satisfies all 8 interface methods (the §11.0 commons
// quintet via Name/Capabilities/Send/Subscribe/HealthCheck plus the three
// Wave 7 inbound-runtime methods). The reply method is named
// SendReplyGeneric — matching the channels.Channel interface as resolved by
// Wave 7 T1 (NOT SendReply; see channel.go package doc divergence note).
type fakeChannel struct{ name string }

func (f *fakeChannel) Name() string                       { return f.name }
func (f *fakeChannel) Capabilities() commons.Capabilities { return commons.Capabilities{Text: true} }
func (f *fakeChannel) Send(context.Context, commons.OutboundMessage) (commons.Receipt, error) {
	return commons.Receipt{}, nil
}
func (f *fakeChannel) Subscribe(context.Context, commons.InboundHandler) error { return nil }
func (f *fakeChannel) HealthCheck(context.Context) error                       { return nil }
func (f *fakeChannel) SendReplyGeneric(context.Context, commons.Recipient, string, string, []commons.Attachment) (string, error) {
	return "", nil
}
func (f *fakeChannel) BotSelfIdentity(context.Context) (channels.SelfIdentity, error) {
	return channels.SelfIdentity{}, nil
}
func (f *fakeChannel) DownloadAttachment(context.Context, string, string) (string, string, error) {
	return "", "", nil
}

func TestRegistryResolvesRegisteredChannel(t *testing.T) {
	channels.Register("fake-rt", func(channels.Config) (channels.Channel, error) {
		return &fakeChannel{name: "fake-rt"}, nil
	})
	c, err := channels.New("fake-rt", channels.Config{})
	if err != nil {
		t.Fatalf("New(fake-rt): %v", err)
	}
	if c.Name() != "fake-rt" {
		t.Fatalf("Name()=%q want fake-rt", c.Name())
	}
}

func TestRegistryUnknownChannelErrors(t *testing.T) {
	_, err := channels.New("does-not-exist", channels.Config{})
	if err == nil || !errors.Is(err, channels.ErrUnknownChannel) {
		t.Fatalf("err=%v want ErrUnknownChannel", err)
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate Register should panic")
		}
	}()
	channels.Register("dup-rt", func(channels.Config) (channels.Channel, error) { return nil, nil })
	channels.Register("dup-rt", func(channels.Config) (channels.Channel, error) { return nil, nil })
}

func TestTgramRegisteredViaInit(t *testing.T) { // blank import above triggers init()
	for _, n := range channels.Names() {
		if n == string(commons.ChannelTelegram) {
			return
		}
	}
	t.Fatalf("tgram not registered via init(); Names()=%v", channels.Names())
}
