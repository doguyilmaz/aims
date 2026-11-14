import SwiftUI
import UsageCore

struct GaugeAccountRow: View {
    let provider: UsageProvider
    let row: UsageRow
    let refreshFailed: Bool

    var body: some View {
        TimelineView(.periodic(from: .now, by: 60)) { context in
            let stale =
                refreshFailed || row.isStale(at: context.date)
                || row.error != nil || provider.error != nil
            HStack(alignment: .center, spacing: 10) {
                VStack(alignment: .leading, spacing: 1) {
                    Text(ProviderStyle.accountName(row))
                        .font(.system(size: 11, weight: .medium))
                        .lineLimit(1).truncationMode(.middle).help(row.name)
                    HStack(spacing: 4) {
                        if row.bankedResets != nil {
                            UsageReadingDetails(provider: provider, row: row, labelStyle: .compactBanked)
                        } else if let plan = ProviderStyle.planLabel(row.plan) {
                            Text(plan).lineLimit(1).help(plan)
                        }
                        if stale {
                            Image(systemName: "clock").help("Cached reading; refresh for current usage")
                                .accessibilityLabel("Cached reading")
                        }
                        UsageReadingDetails(provider: provider, row: row)
                    }.font(.system(size: 10)).foregroundStyle(.secondary)
                }.frame(width: 96, alignment: .leading)
                if let windows = row.windows, !windows.isEmpty {
                    LazyVGrid(
                        columns: [GridItem(.flexible(), spacing: 8), GridItem(.flexible(), spacing: 8)],
                        alignment: .leading, spacing: 6
                    ) {
                        ForEach(Array(windows.enumerated()), id: \.offset) { _, window in
                            GaugeMetric(
                                window: window, tint: ProviderStyle.tint(provider.id), stale: stale, now: context.date)
                        }
                    }.frame(maxWidth: .infinity, alignment: .leading)
                } else {
                    Text("Limits not reported")
                        .font(.system(size: 11)).foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }.frame(maxWidth: .infinity, minHeight: 32, alignment: .leading)
        }
    }

}
