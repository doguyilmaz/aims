import AppKit
import SwiftUI
import UsageCore

struct UsageDashboard: View {
    @Bindable var model: UsageModel
    let togglePanel: @MainActor () -> Void
    @Environment(\.openSettings) private var openSettings
    @State private var editingReminder = false

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider().opacity(0.5)
            ScrollView {
                LazyVStack(spacing: 12) {
                    if model.fixtureURL != nil {
                        Label("Preview data", systemImage: "paintpalette")
                            .font(.caption).foregroundStyle(.secondary).frame(maxWidth: .infinity, alignment: .leading)
                    }
                    if let error = model.error {
                        Label(error, systemImage: "exclamationmark.triangle")
                            .font(.caption).foregroundStyle(.orange).frame(maxWidth: .infinity, alignment: .leading)
                    }
                    if let snapshot = model.snapshot {
                        if let error = snapshot.customResetsError {
                            Label(error, systemImage: "exclamationmark.triangle").font(.caption).foregroundStyle(
                                .orange)
                        }
                        ForEach(snapshot.providers) { provider in ProviderCard(provider: provider) }
                        if let reminders = snapshot.customResets, !reminders.isEmpty {
                            VStack(alignment: .leading, spacing: 12) {
                                Label("CUSTOM REMINDERS", systemImage: "calendar.badge.clock").font(
                                    .system(size: 10, weight: .semibold)
                                ).foregroundStyle(.mint)
                                ForEach(reminders) { reminder in
                                    HStack(alignment: .top) {
                                        VStack(alignment: .leading, spacing: 4) {
                                            Text(reminder.label).font(.system(size: 12, weight: .medium))
                                            Text("\(reminder.provider) / \(reminder.row)").font(.caption2)
                                                .foregroundStyle(.secondary)
                                            Text(reminder.at, format: .dateTime.month().day().hour().minute()).font(
                                                .caption2)
                                            Text("User reminder · quota unchanged").font(.system(size: 9))
                                                .foregroundStyle(.tertiary)
                                        }
                                        Spacer()
                                        Button {
                                            Task { await model.removeReminder(id: reminder.id) }
                                        } label: {
                                            Image(systemName: "trash")
                                        }
                                        .buttonStyle(.borderless).disabled(
                                            model.isRefreshing || model.fixtureURL != nil
                                        ).help("Remove reminder")
                                    }
                                }
                            }.padding(14).frame(maxWidth: .infinity, alignment: .leading)
                                .background(.mint.opacity(0.06), in: RoundedRectangle(cornerRadius: 16))
                        }
                    } else {
                        ContentUnavailableView(
                            "Your limits, together", systemImage: "gauge.with.dots.needle.50percent",
                            description: Text("Reading the aims usage cache…")
                        )
                        .padding(.vertical, 60)
                    }
                }.padding(16)
            }
            footer
        }
        .background(.regularMaterial)
        .preferredColorScheme(.dark)
        .sheet(isPresented: $editingReminder) { CustomResetEditor(model: model) }
    }

    private var header: some View {
        HStack(spacing: 10) {
            Image(systemName: "circle.hexagongrid.fill").font(.title2).foregroundStyle(.mint)
            VStack(alignment: .leading, spacing: 2) {
                Text("AIMS").font(.system(.headline, design: .rounded).weight(.bold)).tracking(2)
                Text("Usage & resets").font(.caption).foregroundStyle(.secondary)
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
            }.disabled(model.isRefreshing).help("Refresh without sending a model prompt")
            Button(action: togglePanel) { Image(systemName: model.panelVisible ? "pin.fill" : "pin") }
                .help(model.panelVisible ? "Close right-edge panel" : "Pin to the right edge of this screen")
                .accessibilityLabel("Toggle right-edge panel")
            Menu {
                Button("Add custom reset reminder…") { editingReminder = true }
                    .disabled(
                        model.fixtureURL != nil || model.snapshot?.providers.allSatisfy { $0.rows.isEmpty } != false)
                Button("Settings…") {
                    openSettings()
                    NSApp.activate(ignoringOtherApps: true)
                }
                Divider()
                Button("Quit Aims Usage", role: .destructive) { NSApp.terminate(nil) }.keyboardShortcut("q")
            } label: {
                Image(systemName: "ellipsis")
            }.menuStyle(.borderlessButton).frame(width: 20)
        }
        .buttonStyle(.borderless)
        .padding(.horizontal, 18).padding(.vertical, 16)
    }

    private var footer: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Circle().fill(model.isRefreshing ? .orange : .mint).frame(width: 5, height: 5)
                Text(
                    model.isRefreshing
                        ? "Refreshing providers…"
                        : "\(model.automaticRefresh ? "Every 5 min" : "Manual refresh") · local time")
                Spacer()
                if let time = model.snapshot?.generatedAt { Text(time, style: .time).monospacedDigit() }
            }
            Text("Claude updates from its last reported status line.")
                .foregroundStyle(.tertiary)
        }
        .font(.system(size: 10)).foregroundStyle(.secondary)
        .padding(.horizontal, 18).padding(.vertical, 12)
        .background(.black.opacity(0.12))
    }
}
