import SwiftUI
import UsageCore

struct EdgeDockView: View {
    @Bindable var model: UsageModel
    let maximumHeight: CGFloat
    let hoverProvider: @MainActor (String) -> Void
    let selectProvider: @MainActor (String) -> Void
    let openOverview: @MainActor () -> Void
    @Environment(\.colorScheme) private var scheme

    var body: some View {
        VStack(spacing: 8) {
            if let providers = model.snapshot?.displayProviders {
                ScrollView {
                    VStack(spacing: 4) {
                        ForEach(providers) { provider in
                            TimelineView(.periodic(from: .now, by: 60)) { context in
                                let summary = DockSummary(
                                    provider: provider, now: context.date, globalError: model.error)
                                Button {
                                    selectProvider(provider.id)
                                } label: {
                                    VStack(spacing: 4) {
                                        ZStack {
                                            Circle().stroke(ProviderStyle.tint(provider.id).opacity(0.14), lineWidth: 3)
                                            if let percent = summary.percent {
                                                Circle().trim(from: 0, to: min(1, percent / 100))
                                                    .stroke(
                                                        ProviderStyle.tint(provider.id),
                                                        style: StrokeStyle(lineWidth: 3, lineCap: .round)
                                                    )
                                                    .rotationEffect(.degrees(-90))
                                            }
                                            ProviderMark(id: provider.id, size: 19)
                                        }.frame(width: 38, height: 38)
                                        Text(summary.value).font(.system(size: 12, weight: .semibold)).monospacedDigit()
                                            .foregroundStyle(summary.blocked ? .orange : .primary).lineLimit(1)
                                    }
                                    .frame(width: 54, height: 60)
                                    .background(
                                        ProviderStyle.tint(provider.id).opacity(
                                            model.dockProvider == provider.id ? 0.13 : 0),
                                        in: RoundedRectangle(cornerRadius: 13)
                                    )
                                    .contentShape(RoundedRectangle(cornerRadius: 13))
                                }
                                .buttonStyle(.plain)
                                .onHover { if $0 { hoverProvider(provider.id) } }
                                .accessibilityLabel("\(ProviderStyle.title(provider)) usage, \(summary.value)")
                                .help(
                                    "\(ProviderStyle.title(provider)) · highest current usage · hover for all accounts, click to pin"
                                )
                            }
                        }
                    }
                }
                .frame(
                    height: min(
                        max(0, CGFloat(providers.count) * EdgeDockLayout.rowStride - 4),
                        max(0, maximumHeight - EdgeDockLayout.chromeHeight - 4))
                )
                .scrollDisabled(
                    CGFloat(providers.count) * EdgeDockLayout.rowStride - 4
                        <= maximumHeight - EdgeDockLayout.chromeHeight - 4
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
            HStack(spacing: 12) {
                Button(action: openOverview) {
                    Image(systemName: model.error == nil ? "square.grid.2x2" : "exclamationmark.triangle")
                        .foregroundStyle(model.error == nil ? Color.secondary : Color.orange).frame(
                            width: 20, height: 20)
                }
                .help(model.error ?? "Open overview").accessibilityLabel("Open overview")
                Button {
                    model.edgeDockEnabled = false
                } label: {
                    Image(systemName: "xmark").frame(width: 20, height: 20)
                }
                .help("Hide Edge Dock").accessibilityLabel("Hide Edge Dock")
            }.buttonStyle(.plain).font(.system(size: 11)).foregroundStyle(.secondary).frame(height: 20)
        }
        .padding(.vertical, EdgeDockLayout.contentInset).frame(width: EdgeDockLayout.width)
        .frame(maxHeight: .infinity)
        .background(UsageBackdrop(inkOpacity: 0.58))
        .clipShape(EdgeDockShape())
        .overlay(EdgeDockShape().stroke(UsagePalette.border(scheme), lineWidth: 1))
    }
}
