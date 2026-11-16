import SwiftUI
import UsageCore

struct DockOptionsView: View {
    @Bindable var model: UsageModel
    let providerID: String?
    let width: CGFloat
    let maximumBodyHeight: CGFloat
    let close: @MainActor () -> Void
    let openOverview: @MainActor () -> Void
    let heightDidChange: @MainActor (CGFloat) -> Void
    @State private var contentHeight: CGFloat = 300

    private var provider: UsageProvider? {
        model.snapshot?.providers.first { $0.id == providerID }
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                if let provider { ProviderMark(id: provider.id, size: 18) }
                Text(provider.map { ProviderStyle.title($0) } ?? "Edge Dock")
                    .font(.system(size: 13, weight: .semibold))
                Spacer()
                UsageIconButton(title: "Close Dock options", icon: .close, action: close)
            }.padding(.horizontal, 14).frame(height: 42)
            Divider()
            ScrollView {
                VStack(alignment: .leading, spacing: 2) {
                    if let provider, model.dockAccount(for: provider.id).canChoose(in: provider) {
                        DockAccountSelector(model: model, provider: provider).padding(.horizontal, 8)
                        Divider().padding(.vertical, 6)
                    }
                    Text("Usage window").font(.system(size: 11)).foregroundStyle(.secondary).padding(.horizontal, 8)
                    if let provider {
                        focusChoices(for: provider)
                    } else {
                        ForEach(model.snapshot?.displayProviders ?? []) { provider in
                            DisclosureGroup {
                                if model.dockAccount(for: provider.id).canChoose(in: provider) {
                                    DockAccountSelector(model: model, provider: provider)
                                }
                                focusChoices(for: provider)
                            } label: {
                                HStack {
                                    Text(ProviderStyle.title(provider))
                                    Spacer()
                                    Text(selectionTitle(for: provider)).foregroundStyle(.secondary)
                                        .multilineTextAlignment(.trailing)
                                }
                            }.font(.system(size: 12)).padding(.horizontal, 8).padding(.vertical, 4)
                        }
                    }
                    Divider().padding(.vertical, 6)
                    Text("Percentages").font(.system(size: 11)).foregroundStyle(.secondary).padding(.horizontal, 8)
                    ForEach(DockValueMode.allCases) { mode in
                        DockOptionButton(title: mode.title, selected: model.edgeDockValueMode == mode) {
                            model.edgeDockValueMode = mode
                            close()
                        }
                    }
                    Divider().padding(.vertical, 6)
                    DockOptionButton(title: "Pin Edge Dock", selected: model.edgeDockPinned) {
                        model.edgeDockPinned.toggle()
                        close()
                    }
                    DockOptionButton(title: "Open overview") {
                        close()
                        openOverview()
                    }
                    DockOptionButton(title: "Hide Edge Dock") {
                        close()
                        model.edgeDockEnabled = false
                    }
                }
                .padding(8).fixedSize(horizontal: false, vertical: true)
                .background {
                    GeometryReader { geometry in
                        Color.clear.preference(key: ContentHeightKey.self, value: geometry.size.height)
                    }
                }
            }
            .frame(height: min(contentHeight, maximumBodyHeight))
            .scrollDisabled(contentHeight <= maximumBodyHeight + 1)
            .scrollIndicators(contentHeight > maximumBodyHeight + 1 ? .automatic : .hidden)
        }
        .frame(width: width)
        .usagePanelSurface(in: RoundedRectangle(cornerRadius: 12))
        .background(UsageScrollStyle()).environment(\.usageSlimScrollbars, model.slimScrollbars)
        .onPreferenceChange(ContentHeightKey.self) { value in
            guard value > 0 else { return }
            if abs(contentHeight - value) > 0.5 { contentHeight = value }
            heightDidChange(min(value, maximumBodyHeight) + 43)
        }
        .onExitCommand(perform: close)
    }

    private func focusChoices(for provider: UsageProvider) -> some View {
        let selected = model.dockFocus(for: provider.id)
        let reported = DockWindowFocus.options(for: provider)
        return ForEach(DockWindowFocus.options(for: provider, including: selected)) { focus in
            DockOptionButton(
                title: focus.title + (reported.contains(focus) ? "" : " · Not reported"), selected: focus == selected
            ) {
                model.setDockFocus(focus, for: provider.id)
                close()
            }
        }
    }

    private func selectionTitle(for provider: UsageProvider) -> String {
        let window = model.dockFocus(for: provider.id).title
        let account = model.dockAccount(for: provider.id)
        return account == .all ? window : account.title(in: provider) + " · " + window
    }
}
