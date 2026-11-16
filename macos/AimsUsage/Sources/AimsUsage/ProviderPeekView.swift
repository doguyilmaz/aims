import SwiftUI

struct ProviderPeekView: View {
    @Bindable var model: UsageModel
    let providerID: String
    let maximumBodyHeight: CGFloat
    let close: @MainActor () -> Void
    let togglePin: @MainActor () -> Void
    let heightDidChange: @MainActor (CGFloat) -> Void
    @State private var contentHeight: CGFloat = 160

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                if let provider = model.snapshot?.providers.first(where: { $0.id == providerID }) {
                    ProviderMark(id: provider.id, size: 22)
                    if let error = model.error {
                        Image(systemName: "exclamationmark.triangle").foregroundStyle(.orange).help(error)
                    }
                    Text(ProviderStyle.title(provider)).font(.system(size: 15, weight: .semibold))
                    Spacer()
                    let unit = provider.id == "agy" ? "group" : provider.id == "opencode" ? "budget" : "account"
                    Text("\(provider.rows.count) \(unit)\(provider.rows.count == 1 ? "" : "s")")
                        .font(.system(size: 11)).foregroundStyle(.secondary)
                }
                UsageIconButton(title: "Refresh quotas", icon: .refresh, busy: model.isRefreshing) {
                    Task { await model.refresh() }
                }.disabled(model.isRefreshing).help("Refresh quotas without a model prompt")
                UsageIconButton(
                    title: model.dockPeekPinned ? "Unpin provider details" : "Pin provider details",
                    icon: model.dockPeekPinned ? .pinned : .pin, selected: model.dockPeekPinned, action: togglePin)
                UsageIconButton(title: "Close provider details", icon: .close, action: close)
            }.padding(.horizontal, 18).frame(height: 50)
            ScrollView {
                if let provider = model.snapshot?.providers.first(where: { $0.id == providerID }) {
                    VStack(alignment: .leading, spacing: 12) {
                        if model.dockAccount(for: provider.id).canChoose(in: provider) {
                            DockAccountSelector(model: model, provider: provider, label: "Dock")
                        }
                        CompactProviderCard(provider: provider, showHeading: false, refreshFailed: model.error != nil)
                    }
                    .padding(18)
                    .background {
                        GeometryReader { geometry in
                            Color.clear.preference(key: ContentHeightKey.self, value: geometry.size.height)
                        }
                    }
                }
            }
            .frame(height: min(contentHeight, maximumBodyHeight))
            .scrollDisabled(contentHeight <= maximumBodyHeight + 1)
            .scrollIndicators(contentHeight > maximumBodyHeight + 1 ? .automatic : .hidden)
            .simultaneousGesture(
                TapGesture().onEnded {
                    if model.dockProvider == providerID { model.dockPeekPinned = true }
                })
            HStack(spacing: 4) {
                Text("% used ·")
                if model.isRefreshing {
                    Text("Refreshing…")
                } else if let provider = model.snapshot?.providers.first(where: { $0.id == providerID }),
                    let oldest = provider.rows.compactMap(\.observedAt).min()
                {
                    Text("Data age")
                    Text(oldest, style: .relative)
                        .help("Oldest reported update: \(oldest.formatted(date: .abbreviated, time: .shortened))")
                } else {
                    Text("Update time unavailable")
                }
                Spacer(minLength: 0)
            }.font(.system(size: 11)).foregroundStyle(.secondary)
                .padding(.horizontal, 18).frame(height: 30)
        }
        .frame(width: EdgeDockLayout.peekWidth)
        .background(UsageBackdrop(inkOpacity: 0.72)).clipShape(RoundedRectangle(cornerRadius: 18))
        .background(UsageScrollStyle())
        .onPreferenceChange(ContentHeightKey.self) { value in
            guard value > 0 else { return }
            if abs(contentHeight - value) > 0.5 { contentHeight = value }
            heightDidChange(min(value, maximumBodyHeight) + 80)
        }
        .onChange(of: maximumBodyHeight) { heightDidChange(min(contentHeight, maximumBodyHeight) + 80) }
        .onExitCommand(perform: close)
        .environment(\.usageSlimScrollbars, model.slimScrollbars)
    }
}
