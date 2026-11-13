import AppKit
import SwiftUI
import UsageCore

struct UsageDashboard: View {
    @Bindable var model: UsageModel
    let togglePanel: @MainActor () -> Void
    let surface: UsageSurface
    var width: CGFloat = 400
    var maximumBodyHeight: CGFloat = 1_100
    var heightDidChange: @MainActor (CGFloat) -> Void = { _ in }
    @Environment(\.openSettings) private var openSettings
    @Environment(\.openWindow) private var openWindow
    @State private var editingReminder = false
    @State private var showingReminders = false
    @State private var contentHeight: CGFloat = 320

    var body: some View {
        VStack(spacing: 0) {
            header.frame(height: 52)
            ScrollView {
                content.padding(16)
                    .background {
                        GeometryReader { geometry in
                            Color.clear.preference(key: ContentHeightKey.self, value: geometry.size.height)
                        }
                    }
            }
            .frame(height: min(contentHeight, maximumBodyHeight))
            .scrollDisabled(contentHeight <= maximumBodyHeight + 1)
            .scrollIndicators(contentHeight > maximumBodyHeight + 1 ? .automatic : .hidden)
            .scrollBounceBehavior(.basedOnSize)
            .id(model.displayStyle.rawValue)
            footer.frame(height: 32)
        }
        .onPreferenceChange(ContentHeightKey.self) { value in
            guard value > 0 else { return }
            if abs(contentHeight - value) > 0.5 { contentHeight = value }
            heightDidChange(min(value, maximumBodyHeight) + 84)
        }
        .onChange(of: maximumBodyHeight) { heightDidChange(min(contentHeight, maximumBodyHeight) + 84) }
        .frame(width: width)
        .background(UsageBackdrop())
        .background(UsageScrollStyle())
        .sheet(isPresented: $editingReminder) { CustomResetEditor(model: model) }
        .environment(\.usageSlimScrollbars, model.slimScrollbars)
    }

    @ViewBuilder private var content: some View {
        VStack(alignment: .leading, spacing: 14) {
            if let error = model.error { notice(error) }
            if let error = model.snapshot?.customResetsError { notice(error) }
            if let snapshot = model.snapshot {
                if model.displayStyle == .compact {
                    VStack(spacing: maximumBodyHeight < 850 ? 6 : 12) {
                        ForEach(snapshot.displayProviders) { provider in
                            CompactProviderCard(provider: provider, refreshFailed: model.error != nil)
                            if provider.id != snapshot.displayProviders.last?.id {
                                Divider().opacity(0.4)
                            }
                        }
                    }
                } else if surface == .popover {
                    PopoverGaugeList(snapshot: snapshot, refreshFailed: model.error != nil)
                } else {
                    let items = UsageTileItem.items(from: snapshot)
                    let twoColumnRows = UsageGaugeRow.rows(from: items, columns: 2)
                    let columns = width >= 520 && UsageGaugeRow.contentHeight(twoColumnRows) > maximumBodyHeight ? 3 : 2
                    let rows = columns == 2 ? twoColumnRows : UsageGaugeRow.rows(from: items, columns: 3)
                    let tileWidth = (width - 32 - CGFloat(columns - 1) * 10) / CGFloat(columns)
                    VStack(spacing: 10) {
                        ForEach(rows) { row in
                            HStack(alignment: .top, spacing: 10) {
                                ForEach(row.items) { item in
                                    UsageAccountTile(
                                        provider: item.provider, row: item.row, metricRows: row.metricRows,
                                        refreshFailed: model.error != nil
                                    )
                                    .frame(width: tileWidth, height: row.height)
                                }
                                if row.items.count < columns { Spacer(minLength: 0) }
                            }.frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                }
            } else if model.error != nil {
                ContentUnavailableView(
                    "Usage unavailable", systemImage: "exclamationmark.triangle",
                    description: Text("Check the data folder and aims executable in Settings.")
                )
                .frame(maxWidth: .infinity).padding(.vertical, 30)
            } else {
                ContentUnavailableView(
                    "Reading your limits", systemImage: "gauge.with.dots.needle.50percent",
                    description: Text("Loading the aims usage cache…")
                )
                .frame(maxWidth: .infinity).padding(.vertical, 30)
            }
        }
    }

    private var header: some View {
        HStack(spacing: 8) {
            Text("aims").font(.system(size: 17, weight: .bold)).tracking(-0.5)
                .foregroundStyle(.primary)
            Spacer(minLength: 8)
            Button {
                Task { await model.refresh() }
            } label: {
                if model.isRefreshing {
                    ProgressView().controlSize(.small).frame(width: 18, height: 18)
                } else {
                    Image(systemName: "arrow.clockwise")
                }
            }.disabled(model.isRefreshing).help("Refresh quotas without a model prompt")
            Button(action: togglePanel) {
                Image(systemName: surface == .panel ? "xmark" : model.panelVisible ? "pin.fill" : "pin")
            }.help(surface == .panel ? "Close panel" : "Pin this overview")
                .accessibilityLabel("Toggle overview panel")
            Menu {
                Button(model.edgeDockEnabled ? "Hide Edge Dock" : "Show Edge Dock", systemImage: "sidebar.right") {
                    model.edgeDockEnabled.toggle()
                }
                Button("Add custom reset reminder…") { editingReminder = true }
                    .disabled(
                        model.fixtureURL != nil || model.snapshot?.providers.allSatisfy { $0.rows.isEmpty } != false)
                Button("Settings…") { showSettings() }
                if Bundle.main.object(forInfoDictionaryKey: "AimsUsagePreview") as? Bool == true {
                    Button("Preview menu layout…") { openWindow(id: "usage-menu-preview") }
                }
                #if AIMS_USAGE_PREVIEW
                    Button("Preview small-screen overflow…") { openWindow(id: "usage-small-preview") }
                    Button("Toggle preview appearance") {
                        let dark = NSApp.effectiveAppearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
                        NSApp.appearance = NSAppearance(named: dark ? .aqua : .darkAqua)
                    }
                #endif
                Divider()
                Text(
                    "Aims Usage \(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.2.0")"
                )
                Text(
                    "Build \(Bundle.main.object(forInfoDictionaryKey: "AimsSourceRevision") as? String ?? "development")"
                )
                Button("Quit Aims Usage", role: .destructive) { NSApp.terminate(nil) }.keyboardShortcut("q")
            } label: {
                Image(systemName: "ellipsis")
            }
            .menuStyle(.borderlessButton).frame(width: 22)
        }
        .buttonStyle(.borderless).font(.system(size: 13, weight: .medium))
        .padding(.horizontal, 16)
    }

    private func notice(_ message: String) -> some View {
        Label(message, systemImage: "exclamationmark.triangle")
            .font(.system(size: 12)).foregroundStyle(.orange).lineLimit(1).help(message)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var footer: some View {
        HStack(spacing: 6) {
            Text("% used").fontWeight(.medium)
            Text("·")
            if model.fixtureURL != nil {
                Text("Preview data")
            } else if model.isRefreshing {
                Text("Refreshing…")
            } else {
                Text(model.automaticRefresh ? "Every 5 min" : "Manual refresh")
            }
            Spacer()
            if let reminders = model.snapshot?.customResets, !reminders.isEmpty {
                Button {
                    showingReminders.toggle()
                } label: {
                    Label("\(reminders.count)", systemImage: "calendar.badge.clock")
                }.buttonStyle(.borderless).help("Your reset reminders")
                    .popover(isPresented: $showingReminders) { ReminderListView(model: model).frame(width: 330) }
            }
            Button {
                showSettings()
            } label: {
                Image(systemName: "gearshape")
            }
            .buttonStyle(.borderless).help("Settings").accessibilityLabel("Settings")
        }
        .font(.system(size: 11)).foregroundStyle(.secondary).padding(.horizontal, 16)
    }

    private func showSettings() {
        openSettings()
        NSApp.activate(ignoringOtherApps: true)
    }
}
