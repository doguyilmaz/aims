import SwiftUI
import UsageCore

struct EdgeDockView: View {
    @Bindable var model: UsageModel
    let maximumHeight: CGFloat
    let hoverProvider: @MainActor (String) -> Void
    let selectProvider: @MainActor (String) -> Void
    let openOverview: @MainActor () -> Void
    let showOptions: @MainActor (String?, CGPoint?) -> Void
    private var size: EdgeDockSize { model.edgeDockSize }

    var body: some View {
        VStack(spacing: 8) {
            if let providers = model.snapshot?.displayProviders {
                ScrollView {
                    VStack(spacing: 4) {
                        ForEach(providers) { provider in
                            TimelineView(.periodic(from: .now, by: 60)) { context in
                                let summary = DockSummary(
                                    provider: provider, now: context.date, globalError: model.error,
                                    focus: model.dockFocus(for: provider.id), valueMode: model.edgeDockValueMode,
                                    account: model.dockAccount(for: provider.id))
                                Button {
                                    selectProvider(provider.id)
                                } label: {
                                    VStack(spacing: size == .mini ? 2 : 4) {
                                        ZStack {
                                            Circle().stroke(ProviderStyle.tint(provider.id).opacity(0.14), lineWidth: 3)
                                            if let percent = summary.displayPercent {
                                                Circle().trim(from: 0, to: min(1, percent / 100))
                                                    .stroke(
                                                        ProviderStyle.tint(provider.id),
                                                        style: StrokeStyle(lineWidth: 3, lineCap: .round)
                                                    )
                                                    .rotationEffect(.degrees(-90))
                                                    .opacity(summary.isCached ? 0.5 : 1)
                                            }
                                            ProviderMark(id: provider.id, size: size.iconSize)
                                        }.frame(width: size.ringSize, height: size.ringSize)
                                            .overlay(alignment: .bottomTrailing) {
                                                if summary.isCached {
                                                    Image(systemName: "clock.fill")
                                                        .font(.system(size: size == .mini ? 8 : 10))
                                                        .foregroundStyle(.orange).padding(1)
                                                        .background(.background, in: Circle())
                                                }
                                            }
                                        Text(size == .mini && summary.blocked ? "!" : summary.value)
                                            .font(.system(size: size.labelSize, weight: .semibold))
                                            .monospacedDigit()
                                            .foregroundStyle(
                                                summary.blocked
                                                    ? Color.orange : summary.isCached ? .secondary : .primary
                                            ).lineLimit(1)
                                    }
                                    .frame(width: size.width - 10, height: size.rowStride - 4)
                                    .background(
                                        ProviderStyle.tint(provider.id).opacity(
                                            model.dockProvider == provider.id ? 0.13 : 0),
                                        in: RoundedRectangle(cornerRadius: 13)
                                    )
                                    .contentShape(RoundedRectangle(cornerRadius: 13))
                                }
                                .buttonStyle(.plain)
                                .onHover { if $0 { hoverProvider(provider.id) } }
                                .overlay {
                                    DockContextMenu { showOptions(provider.id, $0) }
                                        .accessibilityHidden(true)
                                }
                                .accessibilityAction(named: Text("Dock options")) { showOptions(provider.id, nil) }
                                .accessibilityLabel(
                                    "\(ProviderStyle.title(provider)) usage, \(summary.value), \(summary.explanation)"
                                )
                                .help(
                                    "\(ProviderStyle.title(provider)) · \(summary.value) \(model.edgeDockValueMode.title.lowercased()) · \(summary.explanation) · right-click to customize"
                                )
                            }
                        }
                    }
                }
                .frame(
                    height: min(
                        max(0, CGFloat(providers.count) * size.rowStride - 4),
                        max(0, maximumHeight - size.chromeHeight - 4))
                )
                .scrollDisabled(
                    CGFloat(providers.count) * size.rowStride - 4
                        <= maximumHeight - size.chromeHeight - 4
                )
                .scrollIndicators(.hidden)
            } else if let error = model.error {
                Button(action: openOverview) {
                    VStack(spacing: 6) {
                        Image(systemName: "exclamationmark.triangle")
                        Text("Check")
                    }
                    .font(.system(size: 12)).foregroundStyle(.orange).frame(height: 50)
                }.buttonStyle(.plain).help(error).accessibilityLabel("Usage unavailable; open overview")
            } else {
                ProgressView().controlSize(.small).frame(height: 50)
            }
            HStack(spacing: size == .mini ? 0 : 8) {
                UsageIconButton(
                    title: "Open overview", icon: model.error == nil ? .overview : .warning,
                    tint: model.error == nil ? nil : .orange, width: size == .mini ? 20 : 24, height: 20,
                    action: openOverview
                )
                .help(model.error ?? "Open overview · right-click for Dock options").accessibilityLabel("Open overview")
                .overlay { DockContextMenu { showOptions(nil, $0) }.accessibilityHidden(true) }
                .accessibilityAction(named: Text("Dock options")) { showOptions(nil, nil) }
                UsageIconButton(
                    title: model.edgeDockPinned ? "Unpin Edge Dock" : "Pin Edge Dock",
                    icon: model.edgeDockPinned ? .pinned : .pin, selected: model.edgeDockPinned,
                    width: size == .mini ? 20 : 24, height: 20
                ) {
                    model.edgeDockPinned.toggle()
                }
                .accessibilityValue(model.edgeDockPinned ? "Pinned" : "Unpinned")
            }.frame(height: 20)
        }
        .padding(.vertical, size.contentInset).frame(width: size.width)
        .frame(maxHeight: .infinity)
        .usagePanelSurface(in: EdgeDockShape(side: model.edgeDockSide))
    }
}
