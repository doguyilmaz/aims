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
            Divider().opacity(0.5)
            ScrollView {
                VStack(spacing: 8) {
                    if let error = model.error { notice(error) }
                    if let snapshot = model.snapshot {
                        if let error = snapshot.customResetsError { notice(error) }
                        if model.displayStyle == .compact {
                            ForEach(snapshot.displayProviders) { provider in
                                CompactProviderCard(provider: provider)
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
                }.padding(10)
            }
            .id(model.displayStyle.rawValue)
            footer
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(Color(nsColor: .windowBackgroundColor))
        .sheet(isPresented: $editingReminder) { CustomResetEditor(model: model) }
    }

    private var header: some View {
        HStack(spacing: 9) {
            Text("Limits").font(.system(size: 13, weight: .semibold))
            Spacer(minLength: 0)
            Picker("Display style", selection: $model.displayStyle) {
                ForEach(UsageDisplayStyle.allCases) { style in Text(style.title).tag(style) }
            }
            .pickerStyle(.segmented).controlSize(.small).labelsHidden().frame(width: 150)
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
            Button(action: togglePanel) {
                Image(systemName: surface == .panel ? "xmark" : model.panelVisible ? "pin.fill" : "pin")
            }
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
        .padding(.horizontal, 12).padding(.vertical, 10)
    }

    @ViewBuilder private func providerWidgets(_ snapshot: UsageSnapshot) -> some View {
        LazyVGrid(
            columns: [GridItem(.flexible(), alignment: .top), GridItem(.flexible(), alignment: .top)],
            alignment: .leading, spacing: 8
        ) {
            ForEach(UsageTileItem.items(from: snapshot)) { item in
                UsageAccountTile(provider: item.provider, row: item.row)
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
            Text("% used").fontWeight(.medium)
            Text("·")
            Text(
                model.isRefreshing ? "Refreshing…" : model.automaticRefresh ? "Every 5 min" : "Manual refresh"
            )
            Spacer()
            if model.fixtureURL != nil { Text("Preview data") }
            Button {
                showSettings()
            } label: {
                Image(systemName: "gearshape")
            }
            .buttonStyle(.borderless).help("Settings").accessibilityLabel("Settings")
        }
        .font(.system(size: 11)).foregroundStyle(.secondary)
        .padding(.horizontal, 12).padding(.vertical, 8)
    }

    private func showSettings() {
        openSettings()
        NSApp.activate(ignoringOtherApps: true)
    }
}
