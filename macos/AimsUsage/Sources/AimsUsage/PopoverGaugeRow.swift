import SwiftUI
import UsageCore

struct PopoverGaugeRow: View {
    let provider: UsageProvider
    let row: UsageRow?
    let refreshFailed: Bool

    var body: some View {
        TimelineView(.periodic(from: .now, by: 60)) { context in
            let stale =
                refreshFailed || row?.isStale(at: context.date) == true
                || row?.error != nil || provider.error != nil
            HStack(alignment: .top, spacing: 12) {
                VStack(alignment: .leading, spacing: 4) {
                    HStack(spacing: 5) {
                        ProviderMark(id: provider.id, size: 16)
                        Text(ProviderStyle.title(provider))
                            .font(.system(size: 12, weight: .semibold)).lineLimit(1)
                        if let row {
                            UsageReadingDetails(provider: provider, row: row)
                        } else if let error = provider.error {
                            Image(systemName: "exclamationmark.circle").foregroundStyle(.orange).help(error)
                        }
                    }
                    HStack(spacing: 5) {
                        Text(row.map(ProviderStyle.accountName) ?? emptyLabel)
                            .lineLimit(1).truncationMode(.middle).help(row?.name ?? emptyLabel)
                        if let row, row.bankedResets != nil {
                            UsageReadingDetails(provider: provider, row: row, labelStyle: .compactBanked)
                        }
                        if row != nil && stale {
                            Text("Cached").font(.system(size: 10)).foregroundStyle(.secondary)
                                .fixedSize()
                        }
                    }.font(.system(size: 11)).foregroundStyle(.secondary)
                }.frame(width: 128, alignment: .leading)
                if let windows = row?.windows, !windows.isEmpty {
                    LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], alignment: .leading, spacing: 8)
                    {
                        ForEach(Array(windows.enumerated()), id: \.offset) { _, window in
                            PopoverGaugeMetric(
                                window: window, tint: ProviderStyle.tint(provider.id), stale: stale, now: context.date)
                        }
                    }
                } else {
                    Text(row == nil ? emptyLabel : "Limits not reported")
                        .font(.system(size: 11)).foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            .frame(maxWidth: .infinity, minHeight: 36, alignment: .leading).padding(.vertical, 8)
        }
    }

    private var emptyLabel: String {
        provider.id == "claude" || provider.id == "codex"
            ? "No accounts" : provider.installed ? "No reading" : "CLI unavailable"
    }
}
