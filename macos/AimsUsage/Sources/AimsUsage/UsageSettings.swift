import AppKit
import SwiftUI

struct UsageSettings: View {
    @Bindable var model: UsageModel
    @State private var section: UsageSettingsSection? = .general
    @State private var configHome = ""
    @State private var additionalPaths = ""

    private var selectedSection: UsageSettingsSection { section ?? .general }
    private var version: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.3.0"
    }

    var body: some View {
        NavigationSplitView {
            VStack(alignment: .leading, spacing: 0) {
                List(UsageSettingsSection.allCases, selection: $section) { section in
                    NavigationLink(value: section) {
                        Label(section.title, systemImage: section.symbol)
                            .frame(maxWidth: .infinity, alignment: .leading).padding(.vertical, 3)
                    }
                }.listStyle(.sidebar).scrollDisabled(true)
                HStack(spacing: 8) {
                    Image(nsImage: AimsBrandMark.menuImage).frame(width: 20, height: 20)
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Aims Usage").font(.system(size: 11, weight: .medium))
                        Text(version).font(.system(size: 10)).foregroundStyle(.secondary)
                    }
                }.padding(16)
            }.navigationSplitViewColumnWidth(min: 160, ideal: 168, max: 190)
        } detail: {
            Group {
                switch selectedSection {
                case .general: generalSettings
                case .dock: EdgeDockSettings(model: model)
                case .cli: cliSettings
                case .updates: updateSettings
                }
            }.frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                .navigationTitle(selectedSection.title)
                .navigationSubtitle(selectedSection.subtitle)
                .toolbar {
                    if selectedSection == .dock {
                        ToolbarItem(placement: .primaryAction) {
                            Toggle("Show Edge Dock", isOn: $model.edgeDockEnabled)
                                .toggleStyle(.switch).labelsHidden().controlSize(.small)
                                .help("Show Edge Dock")
                        }
                    }
                }
        }
        .navigationSplitViewStyle(.balanced)
        .toolbar(removing: .sidebarToggle)
        .usageSettingsToolbar()
        .background(UsageScrollStyle())
        .environment(\.usageSlimScrollbars, model.slimScrollbars)
        .onAppear {
            configHome = model.configHome
            additionalPaths = model.additionalPaths
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
                Toggle("Show highest current usage in menu bar", isOn: $model.showMenuPercent)
                Toggle("Use slim scrollbars", isOn: $model.slimScrollbars)
                    .help(
                        "Use compact overlay indicators. Turn off to follow macOS scrollbar appearance and visibility.")
            }
            Section("Refresh") {
                Toggle("Refresh providers every 5 minutes", isOn: $model.automaticRefresh)
                Text(
                    "Automatic refresh reads provider quotas without a model prompt. Claude Code owns renewal of its login. No credits are bought or resets redeemed."
                )
                .font(.caption).foregroundStyle(.secondary)
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
