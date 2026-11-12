import AppKit
import SwiftUI
import UsageCore

struct UsageDashboard: View {
    @Bindable var model: UsageModel
    let togglePanel: @MainActor () -> Void
    let surface: UsageSurface
    @Environment(\.openSettings) private var openSettings
    @Environment(\.openWindow) private var openWindow
    @State private var editingReminder = false

    var body: some View {
        VStack(spacing: 0) {
            header
            Picker("Display style", selection: $model.displayStyle) {
                ForEach(UsageDisplayStyle.allCases) { style in
                    Label(style.title, systemImage: style.symbol).tag(style)
                }
            }
            .pickerStyle(.segmented).controlSize(.small).labelsHidden()
            .padding(.horizontal, 14).padding(.bottom, 10)
            ScrollView {
                VStack(spacing: 12) {
                    if let error = model.error { notice(error) }
                    if let snapshot = model.snapshot {
                        if let error = snapshot.customResetsError { notice(error) }
                        if model.displayStyle == .compact {
                            ForEach(snapshot.displayProviders) { provider in
                                CompactProviderCard(provider: provider, surface: surface)
                            }
                        } else {
                            providerWidgets(snapshot)
                        }
                        reminders(snapshot)
                    } else {
                        ContentUnavailableView(
                            "Reading your limits", systemImage: "gauge.with.dots.needle.50percent",
                            description: Text("Loading the aims usage cache…")
                        )
                        .padding(.vertical, 40)
                    }
                }.padding(.horizontal, 14).padding(.top, 4).padding(.bottom, 14)
            }
            .id(model.displayStyle.rawValue)
            footer
        }
        .background(.regularMaterial)
        .sheet(isPresented: $editingReminder) { CustomResetEditor(model: model) }
    }

    private var header: some View {
        HStack(spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                Text(surface == .popover ? "Usage" : "Your limits")
                    .font(.system(size: 17, weight: .semibold, design: .rounded))
                if surface == .panel {
                    Text("Accounts & model groups").font(.system(size: 11)).foregroundStyle(.secondary)
                }
            }
            if model.fixtureURL != nil {
                Text("PREVIEW").font(.system(size: 8, weight: .semibold))
                    .padding(.horizontal, 5).padding(.vertical, 3)
                    .background(.secondary.opacity(0.10), in: Capsule()).foregroundStyle(.secondary)
            }
            Spacer()
            Button {
                Task { await model.refresh() }
            } label: {
                if model.isRefreshing {
                    ProgressView().controlSize(.small).frame(width: 16, height: 16)
                } else {
                    Image(systemName: "arrow.clockwise")
                }
            }.disabled(model.isRefreshing).help(
                "Refresh quotas. Claude uses its last native status line; no model prompt is sent.")
            Button(action: togglePanel) { Image(systemName: model.panelVisible ? "pin.fill" : "pin") }
                .help(model.panelVisible ? "Close right-edge panel" : "Pin to the right edge of this screen")
                .accessibilityLabel("Toggle right-edge panel")
            Menu {
                Button("Add custom reset reminder…") { editingReminder = true }
                    .disabled(
                        model.fixtureURL != nil || model.snapshot?.providers.allSatisfy { $0.rows.isEmpty } != false)
                Button("Settings…") { showSettings() }
                if Bundle.main.object(forInfoDictionaryKey: "AimsUsagePreview") as? Bool == true {
                    Button("Preview menu layout…") { openWindow(id: "usage-menu-preview") }
                }
                Divider()
                Button("Quit Aims Usage", role: .destructive) { NSApp.terminate(nil) }.keyboardShortcut("q")
            } label: {
                Image(systemName: "ellipsis")
            }
            .menuStyle(.borderlessButton).frame(width: 18)
        }
        .buttonStyle(.borderless).font(.system(size: 12)).foregroundStyle(.primary)
        .padding(.horizontal, 18).padding(.top, surface == .panel ? 20 : 15).padding(.bottom, 12)
    }

    @ViewBuilder private func providerWidgets(_ snapshot: UsageSnapshot) -> some View {
        if surface == .panel {
            ForEach(snapshot.displayProviders.filter { $0.id == "claude" || $0.id == "codex" }) { provider in
                ProviderCard(provider: provider, surface: surface)
            }
        } else {
            pairedWidgets(snapshot, ids: ["claude", "codex"])
        }
        pairedWidgets(snapshot, ids: ["kiro", "grok"])
        ForEach(snapshot.displayProviders.filter { !["claude", "codex", "kiro", "grok"].contains($0.id) }) { provider in
            ProviderCard(provider: provider, surface: surface)
        }
    }

    private func pairedWidgets(_ snapshot: UsageSnapshot, ids: [String]) -> some View {
        HStack(alignment: .top, spacing: 10) {
            ForEach(ids, id: \.self) { id in
                if let provider = snapshot.displayProviders.first(where: { $0.id == id }) {
                    ProviderCard(provider: provider, surface: surface)
                }
            }
        }
    }

    @ViewBuilder private func reminders(_ snapshot: UsageSnapshot) -> some View {
        if let reminders = snapshot.customResets, !reminders.isEmpty {
            VStack(alignment: .leading, spacing: 12) {
                Label("Your reminders", systemImage: "calendar.badge.clock")
                    .font(.system(size: 12, weight: .medium))
                ForEach(reminders) { reminder in
                    HStack(alignment: .top, spacing: 10) {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(reminder.label).font(.system(size: 12, weight: .medium))
                            Text(
                                "\(reminder.provider) / \(reminder.row) · \(reminder.at.formatted(date: .abbreviated, time: .shortened))"
                            )
                            .font(.system(size: 10)).foregroundStyle(.secondary)
                        }
                        Spacer(minLength: 0)
                        Button {
                            Task { await model.removeReminder(id: reminder.id) }
                        } label: {
                            Image(systemName: "trash")
                        }.buttonStyle(.borderless).disabled(model.isRefreshing || model.fixtureURL != nil)
                            .help("Remove reminder").accessibilityLabel("Remove reminder \(reminder.label)")
                    }
                }
                Text("Personal reminders · provider quota unchanged").font(.system(size: 10)).foregroundStyle(
                    .secondary)
            }
            .padding(14).frame(maxWidth: .infinity, alignment: .leading)
            .background(.secondary.opacity(0.055), in: RoundedRectangle(cornerRadius: 16))
        }
    }

    private func notice(_ message: String) -> some View {
        Label(message, systemImage: "exclamationmark.triangle")
            .font(.system(size: 11)).foregroundStyle(.orange)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var footer: some View {
        HStack(spacing: 5) {
            Circle().fill(model.isRefreshing ? .orange : .secondary.opacity(0.5)).frame(width: 4, height: 4)
            Text(
                model.isRefreshing ? "Refreshing…" : model.automaticRefresh ? "Refreshes every 5 min" : "Manual refresh"
            )
            Spacer()
            Button {
                showSettings()
            } label: {
                Label(
                    model.configHome.isEmpty
                        ? "Default data" : URL(fileURLWithPath: model.configHome).lastPathComponent,
                    systemImage: "folder"
                )
                .lineLimit(1).truncationMode(.middle)
            }
            .buttonStyle(.borderless)
            .help(model.configHome.isEmpty ? "Data: ~/.aims · Settings" : "Data: \(model.configHome) · Settings")
        }
        .font(.system(size: 10)).foregroundStyle(.secondary)
        .padding(.horizontal, 18).padding(.vertical, 11)
    }

    private func showSettings() {
        openSettings()
        NSApp.activate(ignoringOtherApps: true)
    }
}
