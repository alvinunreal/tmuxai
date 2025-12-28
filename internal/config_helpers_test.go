package internal

import (
	"testing"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/stretchr/testify/assert"
)

func TestGetYolo(t *testing.T) {
	tests := []struct {
		name         string
		configYolo   bool
		sessionYolo  interface{}
		hasOverride  bool
		expectedYolo bool
	}{
		{
			name:         "default false when not configured",
			configYolo:   false,
			hasOverride:  false,
			expectedYolo: false,
		},
		{
			name:         "returns true when Config.Yolo is true",
			configYolo:   true,
			hasOverride:  false,
			expectedYolo: true,
		},
		{
			name:         "session override true takes precedence over config false",
			configYolo:   false,
			sessionYolo:  true,
			hasOverride:  true,
			expectedYolo: true,
		},
		{
			name:         "session override false takes precedence over config true",
			configYolo:   true,
			sessionYolo:  false,
			hasOverride:  true,
			expectedYolo: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &Manager{
				Config: &config.Config{
					Yolo: tt.configYolo,
				},
				SessionOverrides: make(map[string]interface{}),
				LoadedKBs:        make(map[string]string),
			}

			if tt.hasOverride {
				manager.SessionOverrides["yolo"] = tt.sessionYolo
			}

			result := manager.GetYolo()
			assert.Equal(t, tt.expectedYolo, result)
		})
	}
}

func TestConfirmationGettersWithYolo(t *testing.T) {
	tests := []struct {
		name                          string
		yoloEnabled                   bool
		configSendKeysConfirm         bool
		configPasteMultilineConfirm   bool
		configExecConfirm             bool
		expectedSendKeysConfirm       bool
		expectedPasteMultilineConfirm bool
		expectedExecConfirm           bool
	}{
		{
			name:                          "yolo disabled - confirmations respect config true",
			yoloEnabled:                   false,
			configSendKeysConfirm:         true,
			configPasteMultilineConfirm:   true,
			configExecConfirm:             true,
			expectedSendKeysConfirm:       true,
			expectedPasteMultilineConfirm: true,
			expectedExecConfirm:           true,
		},
		{
			name:                          "yolo disabled - confirmations respect config false",
			yoloEnabled:                   false,
			configSendKeysConfirm:         false,
			configPasteMultilineConfirm:   false,
			configExecConfirm:             false,
			expectedSendKeysConfirm:       false,
			expectedPasteMultilineConfirm: false,
			expectedExecConfirm:           false,
		},
		{
			name:                          "yolo enabled - all confirmations return false regardless of config",
			yoloEnabled:                   true,
			configSendKeysConfirm:         true,
			configPasteMultilineConfirm:   true,
			configExecConfirm:             true,
			expectedSendKeysConfirm:       false,
			expectedPasteMultilineConfirm: false,
			expectedExecConfirm:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &Manager{
				Config: &config.Config{
					Yolo:                  tt.yoloEnabled,
					SendKeysConfirm:       tt.configSendKeysConfirm,
					PasteMultilineConfirm: tt.configPasteMultilineConfirm,
					ExecConfirm:           tt.configExecConfirm,
				},
				SessionOverrides: make(map[string]interface{}),
				LoadedKBs:        make(map[string]string),
			}

			assert.Equal(t, tt.expectedSendKeysConfirm, manager.GetSendKeysConfirm())
			assert.Equal(t, tt.expectedPasteMultilineConfirm, manager.GetPasteMultilineConfirm())
			assert.Equal(t, tt.expectedExecConfirm, manager.GetExecConfirm())
		})
	}
}

func TestConfirmationGettersWithSessionOverrides(t *testing.T) {
	t.Run("yolo disabled - session overrides take effect", func(t *testing.T) {
		manager := &Manager{
			Config: &config.Config{
				Yolo:                  false,
				SendKeysConfirm:       true,
				PasteMultilineConfirm: true,
				ExecConfirm:           true,
			},
			SessionOverrides: map[string]interface{}{
				"send_keys_confirm":       false,
				"paste_multiline_confirm": false,
				"exec_confirm":            false,
			},
			LoadedKBs: make(map[string]string),
		}

		assert.False(t, manager.GetSendKeysConfirm())
		assert.False(t, manager.GetPasteMultilineConfirm())
		assert.False(t, manager.GetExecConfirm())
	})

	t.Run("yolo enabled via session - overrides all confirmations", func(t *testing.T) {
		manager := &Manager{
			Config: &config.Config{
				Yolo:                  false,
				SendKeysConfirm:       true,
				PasteMultilineConfirm: true,
				ExecConfirm:           true,
			},
			SessionOverrides: map[string]interface{}{
				"yolo": true,
			},
			LoadedKBs: make(map[string]string),
		}

		assert.False(t, manager.GetSendKeysConfirm())
		assert.False(t, manager.GetPasteMultilineConfirm())
		assert.False(t, manager.GetExecConfirm())
	})
}

func TestYoloSessionOverrideTakesPrecedence(t *testing.T) {
	manager := &Manager{
		Config: &config.Config{
			Yolo:                  false,
			SendKeysConfirm:       true,
			PasteMultilineConfirm: true,
			ExecConfirm:           true,
		},
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}

	assert.True(t, manager.GetSendKeysConfirm())
	assert.True(t, manager.GetPasteMultilineConfirm())
	assert.True(t, manager.GetExecConfirm())
	assert.False(t, manager.GetYolo())

	manager.SessionOverrides["yolo"] = true

	assert.True(t, manager.GetYolo())
	assert.False(t, manager.GetSendKeysConfirm())
	assert.False(t, manager.GetPasteMultilineConfirm())
	assert.False(t, manager.GetExecConfirm())
}
