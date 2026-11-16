import AppKit
import SwiftUI

struct UsageSettings: View {
    @Bindable var model: UsageModel
    @State private var section: UsageSettingsSection? = .general
    @State private var configHome = ""
    @State private var additionalPaths = ""
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var selectedSection: UsageSettingsSection { section ?? .general }
    private var version: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.3.0"
    }

    var body: some View {
        NavigationSplitView {
            List(selection: $section) {
                NavigationLink(value: UsageSettingsSection.general) {
                    SettingsNavigationLabel(section: .general)
                }
                Section("Edge Dock") {
                    ForEach([UsageSettingsSection.dock, .readings]) { section in
                        NavigationLink(value: section) { SettingsNavigationLabel(section: section) }
                    }
                }
                Section("Application") {
                    ForEach([UsageSettingsSection.cli, .updates]) { section in
                        NavigationLink(value: section) { SettingsNavigationLabel(section: section) }
                    }
                }
            }.listStyle(.sidebar).scrollContentBackground(.hidden)
                .safeAreaInset(edge: .bottom, alignment: .leading) {
                    HStack(spacing: 8) {
                        Image(nsImage: AimsBrandMark.menuImage).frame(width: 20, height: 20)
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Aims Usage").font(.caption.weight(.medium))
                            Text(version + " · " + sourceRevision).font(.caption2).foregroundStyle(.secondary)
                        }
                    }.padding(16)
                }.navigationSplitViewColumnWidth(196)
        } detail: {
            VStack(alignment: .leading, spacing: 0) {
                HStack(spacing: 14) {
                    Image(systemName: selectedSection.symbol)
                        .font(.system(size: 28, weight: .medium)).foregroundStyle(selectedSection.tint)
                        .frame(width: 36, height: 42)
                    VStack(alignment: .leading, spacing: 4) {
                        Text(selectedSection.title).font(.title2.weight(.semibold))
                        Text(selectedSection.subtitle).font(.callout).foregroundStyle(.secondary)
                    }
                    Spacer()
                    if selectedSection == .dock {
                        Toggle("Show Edge Dock", isOn: $model.edgeDockEnabled)
                            .toggleStyle(.switch).labelsHidden()
                            .help("Show Edge Dock")
                    }
                }.padding(.horizontal, 24).frame(height: 76)
                ZStack(alignment: .topLeading) {
                    selectedPage.contentMargins(.vertical, 8, for: .scrollContent)
                        .controlSize(.large).toggleStyle(.switch)
                        .id(selectedSection).transition(.opacity)
                }.frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                    .animation(reduceMotion ? nil : .easeInOut(duration: 0.14), value: selectedSection)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
            .navigationTitle("Settings")
        }
        .background(UsageScrollStyle())
        .background(SettingsWindowStyle())
        .environment(\.usageSlimScrollbars, model.slimScrollbars)
        .onAppear {
            NSApp.activate(ignoringOtherApps: true)
            configHome = model.configHome
            additionalPaths = model.additionalPaths
        }
    }

    private var sourceRevision: String {
        let revision = Bundle.main.object(forInfoDictionaryKey: "AimsSourceRevision") as? String ?? "development"
        return String(revision.prefix(7)) + (revision.contains("dirty") ? "+" : "")
    }

    @ViewBuilder private var selectedPage: some View {
        switch selectedSection {
        case .general: generalSettings
        case .dock: EdgeDockSettings(model: model)
        case .readings: DockReadingSettings(model: model)
        case .cli: cliSettings
        case .updates: updateSettings
        }
    }

    private var generalSettings: some View {
        Form {
            Section("Appearance") {
                Picker("Usage appearance", selection: $model.displayStyle) {
                    ForEach(UsageDisplayStyle.allCases) { style in
                        Label(style.title, systemImage: style.symbol).tag(style)
                    }
                }
                Toggle("Show usage in the menu bar", isOn: $model.showMenuPercent)
                Toggle("Use slim scrollbars", isOn: $model.slimScrollbars)
                    .help(
                        "Use compact overlay indicators. Turn off to follow macOS scrollbar appearance and visibility.")
            }
            Section {
                Toggle("Refresh providers every 5 minutes", isOn: $model.automaticRefresh)
            } header: {
                Text("Refresh")
            } footer: {
                Text(
                    "Automatic refresh reads provider quotas without a model prompt. Claude Code owns renewal of its login. No credits are bought or resets redeemed."
                )
            }
        }.formStyle(.grouped)
    }

    private var cliSettings: some View {
        Form {
            TerminalCommandSettings()
            Section("aims executable") {
                TextField("Bundled aims (default)", text: $model.executablePath).textFieldStyle(.roundedBorder)
                HStack {
                    Button("Choose…") { chooseExecutable() }
                    Button("Use bundled") { model.executablePath = "" }
                }
                Text(
                    "Requires aims usage --json support. Provider logins remain in their existing homes; this app stores preferences and selected paths only."
                )
                .font(.caption).foregroundStyle(.secondary)
            }
            Section("aims data folder") {
                TextField("Default: ~/.aims", text: $configHome).textFieldStyle(.roundedBorder)
                HStack {
                    Button("Choose folder…") { chooseDataFolder() }
                    Button("Use default") { configHome = "" }
                }
                Text(
                    "For a custom AIMS_HOME, select the same existing folder you use in Terminal. Finder does not load shell exports. No files are moved."
                )
                .font(.caption).foregroundStyle(.secondary)
            }
            Section("Additional CLI folders") {
                TextEditor(text: $additionalPaths).font(.system(.caption, design: .monospaced)).frame(height: 68)
                Text(
                    "One absolute folder per line; ~/ is supported. These precede the standard Go, Cargo, npm, Homebrew and provider paths. No shell is started."
                )
                .font(.caption).foregroundStyle(.secondary)
                HStack {
                    Spacer()
                    Button("Apply paths") {
                        Task {
                            if await model.applyPaths(configHome: configHome, additionalPaths: additionalPaths) {
                                configHome = model.configHome
                                additionalPaths = model.additionalPaths
                            }
                        }
                    }
                    .disabled(
                        model.isRefreshing
                            || (configHome == model.configHome && additionalPaths == model.additionalPaths))
                }
                if let error = model.error { Text(error).font(.caption).foregroundStyle(.orange) }
            }
        }.formStyle(.grouped)
    }

    private var updateSettings: some View {
        Form {
            Section("App updates") { AppUpdateSettings(updater: model.updater) }
            Section("About") {
                LabeledContent("Aims Usage", value: version)
                LabeledContent(
                    "Build",
                    value: Bundle.main.object(forInfoDictionaryKey: "AimsSourceRevision") as? String ?? "development"
                )
                .font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
            }
        }.formStyle(.grouped)
    }

    private func chooseExecutable() {
        let picker = NSOpenPanel()
        picker.canChooseFiles = true
        picker.canChooseDirectories = false
        picker.allowsMultipleSelection = false
        picker.prompt = "Choose aims"
        if picker.runModal() == .OK, let url = picker.url { model.executablePath = url.path }
    }

    private func chooseDataFolder() {
        let picker = NSOpenPanel()
        picker.canChooseFiles = false
        picker.canChooseDirectories = true
        picker.allowsMultipleSelection = false
        picker.prompt = "Choose aims data"
        if picker.runModal() == .OK, let url = picker.url { configHome = url.path }
    }
}
