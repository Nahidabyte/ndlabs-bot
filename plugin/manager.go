package plugin

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"wa-bot/lib"
)

type Manager struct {
	plugins map[string]Plugin
	mu      sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		plugins: make(map[string]Plugin),
	}
}

func (m *Manager) Register(plugin Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := plugin.GetName()
	if _, exists := m.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	m.plugins[name] = plugin
	return nil
}

func (m *Manager) Unregister(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	if err := plugin.Close(context.Background()); err != nil {
		return fmt.Errorf("failed to close plugin: %w", err)
	}

	delete(m.plugins, name)
	return nil
}

func (m *Manager) GetPlugin(name string) (Plugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", name)
	}

	return plugin, nil
}

func (m *Manager) GetAll() []Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

func (m *Manager) GetAllItems() []lib.PluginItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]lib.PluginItem, 0, len(m.plugins))
	for _, p := range m.plugins {
		items = append(items, p)
	}
	return items
}

func (m *Manager) ExecuteCommand(ctx context.Context, msg *Message, command string, args []string) (Plugin, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, plugin := range m.plugins {
		for _, cmd := range plugin.GetCommands() {
			if strings.EqualFold(cmd, command) {
				response, err := plugin.Execute(ctx, msg, args)
				return plugin, response, err
			}
		}
	}

	return nil, "", fmt.Errorf("command %s not found", command)
}

func (m *Manager) InitAll(ctx context.Context) error {
	m.mu.RLock()
	plugins := make(map[string]Plugin)
	for name, plugin := range m.plugins {
		plugins[name] = plugin
	}
	m.mu.RUnlock()

	for name, plugin := range plugins {
		if err := plugin.Init(ctx); err != nil {
			return fmt.Errorf("failed to initialize plugin %s: %w", name, err)
		}
	}
	return nil
}

func (m *Manager) CloseAll(ctx context.Context) error {
	m.mu.Lock()
	plugins := make(map[string]Plugin)
	for name, plugin := range m.plugins {
		plugins[name] = plugin
	}
	m.mu.Unlock()

	var lastErr error
	for _, plugin := range plugins {
		if err := plugin.Close(ctx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (m *Manager) ListCommands() map[string][]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	commands := make(map[string][]string)
	for _, plugin := range m.plugins {
		commands[plugin.GetName()] = plugin.GetCommands()
	}
	return commands
}
