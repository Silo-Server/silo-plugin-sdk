package runtime

import (
	"context"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/capability"
)

type notificationChannel struct {
	pluginv1.UnimplementedNotificationChannelServer
}

func (notificationChannel) SendNotification(_ context.Context, req *pluginv1.SendNotificationRequest) (*pluginv1.SendNotificationResponse, error) {
	return &pluginv1.SendNotificationResponse{Outcome: pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_DELIVERED, Message: req.GetMessage().GetTitle()}, nil
}

func notificationChannelManifest() *pluginv1.PluginManifest {
	return &pluginv1.PluginManifest{
		PluginId:     "silo.test",
		Capabilities: []*pluginv1.CapabilityDescriptor{{Type: capability.NotificationChannel, Id: "push"}},
	}
}

func TestManifestServeConfigServesNotificationChannel(t *testing.T) {
	cfg, err := manifestServeConfig(notificationChannelManifest(), CapabilityServers{}, WithNotificationChannel(notificationChannel{}))
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}
	resp, err := serveConfigClient(t, cfg).NotificationChannel().SendNotification(context.Background(),
		&pluginv1.SendNotificationRequest{Message: &pluginv1.NotificationMessage{Title: "New episode"}})
	if err != nil || resp.GetMessage() != "New episode" {
		t.Fatalf("SendNotification = %v, %v", resp, err)
	}
}

func TestManifestServeConfigRejectsNotificationChannelWithoutServer(t *testing.T) {
	for name, option := range map[string]ServeManifestOption{
		"no option": nil,
		"typed nil": WithNotificationChannel((*notificationChannel)(nil)),
	} {
		_, err := manifestServeConfig(notificationChannelManifest(), CapabilityServers{}, option)
		if err == nil || !strings.Contains(err.Error(), "WithNotificationChannel") {
			t.Errorf("%s: manifestServeConfig = %v, want an error naming WithNotificationChannel", name, err)
		}
	}
}
