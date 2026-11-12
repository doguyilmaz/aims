import SwiftUI
import UsageCore

struct UsageAccountTile: View {
    let provider: UsageProvider
    let row: UsageRow?
    let metricRows: Int
    var refreshFailed = false
    @Environment(\.colorScheme) private var scheme
    @State private var hovering = false

    var body: some View {
        TimelineView(.periodic(from: .now, by: 60)) { context in
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    ProviderMark(id: provider.id, size: 16)
                    Text(ProviderStyle.title(provider)).font(.system(size: 12, weight: .semibold)).lineLimit(1)
                    Spacer(minLength: 0)
                    if let row {
                        UsageReadingDetails(provider: provider, row: row)
                    } else if let error = provider.error {
                        Image(systemName: "exclamationmark.circle").foregroundStyle(.orange).help(error)
                    }
                }.frame(height: 18)
                Text(row.map(ProviderStyle.accountName) ?? emptyLabel)
                    .font(.system(size: 12)).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
                    .frame(height: 16).help(row?.name ?? emptyLabel)
                Group {
                    if let row, let windows = row.windows, !windows.isEmpty {
                        LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 6) {
                            ForEach(Array(windows.enumerated()), id: \.offset) { _, window in
                                UsageMetricView(
                                    window: window, style: .gauges, tint: ProviderStyle.tint(provider.id),
                                    stale: refreshFailed || row.isStale(at: context.date) || row.error != nil
                                        || provider.error != nil,
                                    now: context.date)
                            }
                        }
                    } else {
                        Text(row == nil ? "Check data and CLI folders in Settings" : "Limits not reported")
                            .font(.system(size: 11)).foregroundStyle(.secondary).lineLimit(2)
                    }
                }.frame(maxWidth: .infinity).frame(
                    height: CGFloat(metricRows) * 74 + CGFloat(metricRows - 1) * 6, alignment: .top)
                HStack(spacing: 4) {
                    if let row,
                        refreshFailed || row.isStale(at: context.date) || row.error != nil || provider.error != nil
                    {
                        Text("Cached").foregroundStyle(.secondary)
                    } else if let windows = row?.windows, !windows.isEmpty,
                        windows.allSatisfy({ $0.validPercent == nil })
                    {
                        Text("Quota unavailable").foregroundStyle(.secondary)
                    } else if let detail = row?.detail, !detail.isEmpty {
                        Text(detail).foregroundStyle(.secondary).help(detail)
                    } else if let plan = ProviderStyle.planLabel(row?.plan) {
                        Text(plan).foregroundStyle(.secondary)
                    }
                    Spacer(minLength: 0)
                    if let row, row.bankedResets != nil {
                        UsageReadingDetails(provider: provider, row: row, labelStyle: .banked)
                    }
                }.font(.system(size: 11)).lineLimit(1).frame(height: 14)
            }
            .padding(10).frame(maxWidth: .infinity)
            .background {
                RoundedRectangle(cornerRadius: 14).fill(UsagePalette.card(scheme))
                RoundedRectangle(cornerRadius: 14).fill(
                    ProviderStyle.tint(provider.id).opacity(hovering ? 0.075 : 0.025))
            }
            .overlay(RoundedRectangle(cornerRadius: 14).strokeBorder(UsagePalette.border(scheme)))
            .onHover { hovering = $0 }
            .animation(.easeOut(duration: 0.12), value: hovering)
        }
    }

    private var emptyLabel: String {
        provider.id == "claude" || provider.id == "codex"
            ? "No accounts" : provider.installed ? "No reading" : "CLI unavailable"
    }
}
