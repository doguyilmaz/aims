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
                Button(action: togglePin) { Image(systemName: model.dockPeekPinned ? "pin.fill" : "pin") }
                    .foregroundStyle(model.dockPeekPinned ? .cyan : .secondary)
                    .help(model.dockPeekPinned ? "Unpin provider details" : "Pin provider details")
                    .accessibilityLabel(model.dockPeekPinned ? "Unpin provider details" : "Pin provider details")
                Button(action: close) { Image(systemName: "xmark") }.help("Close provider details")
                    .accessibilityLabel("Close provider details")
            }.buttonStyle(.borderless).font(.system(size: 12)).padding(.horizontal, 18).frame(height: 50)
            ScrollView {
                if let provider = model.snapshot?.providers.first(where: { $0.id == providerID }) {
                    CompactProviderCard(provider: provider, showHeading: false, refreshFailed: model.error != nil)
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
            Text("% used · reset countdowns").font(.system(size: 11)).foregroundStyle(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading).padding(.horizontal, 18).frame(height: 30)
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
