import SwiftUI
import UsageCore

struct CompactProviderCard: View {
    let provider: UsageProvider
    var showHeading = true
    var refreshFailed = false

    var body: some View {
        TimelineView(.periodic(from: .now, by: 60)) { context in
            VStack(alignment: .leading, spacing: 8) {
                if showHeading {
                    HStack(spacing: 8) {
                        ProviderMark(id: provider.id, size: 20)
                        Text(ProviderStyle.title(provider)).font(.system(size: 13, weight: .semibold))
                        Spacer()
                        if provider.rows.count == 1, let detail = provider.rows.first?.detail, !detail.isEmpty {
                            Text(detail).font(.system(size: 11)).foregroundStyle(.secondary).lineLimit(1).help(detail)
                        }
                        if let error = provider.error, !error.isEmpty {
                            Image(systemName: "exclamationmark.triangle").foregroundStyle(.orange).help(error)
                        }
                    }.frame(height: 22)
                }
                if provider.rows.isEmpty {
                    Text(
                        provider.id == "claude" || provider.id == "codex"
                            ? "No accounts · select your data folder in Settings"
                            : provider.installed ? "No reading yet" : "CLI unavailable"
                    )
                    .font(.system(size: 12)).foregroundStyle(.secondary)
                }
                ForEach(provider.rows) { row in
                    HStack(spacing: 18) {
                        VStack(alignment: .leading, spacing: 4) {
                            HStack(spacing: 4) {
                                Text(ProviderStyle.accountName(row)).font(.system(size: 12, weight: .medium))
                                    .lineLimit(1).truncationMode(.middle).help(row.name)
                                UsageReadingDetails(provider: provider, row: row)
                            }
                            HStack(spacing: 4) {
                                if refreshFailed || row.isStale(at: context.date) || row.error != nil
                                    || provider.error != nil
                                {
                                    Text("Cached")
                                } else if let plan = ProviderStyle.planLabel(row.plan) {
                                    Text(plan)
                                }
                                if row.bankedResets != nil {
                                    UsageReadingDetails(provider: provider, row: row, labelStyle: .compactBanked)
                                }
                            }.font(.system(size: 11)).foregroundStyle(.secondary).lineLimit(1).frame(height: 14)
                        }.frame(width: 128, alignment: .leading)
                        if let windows = row.windows, !windows.isEmpty {
                            LazyVGrid(
                                columns: Array(
                                    repeating: GridItem(.flexible(), spacing: 18), count: min(2, windows.count)),
                                spacing: 10
                            ) {
                                ForEach(Array(windows.enumerated()), id: \.offset) { _, window in
                                    UsageMetricView(
                                        window: window, style: .compact, tint: ProviderStyle.tint(provider.id),
                                        stale: refreshFailed || row.isStale(at: context.date) || row.error != nil
                                            || provider.error != nil,
                                        now: context.date)
                                }
                            }.frame(maxWidth: .infinity)
                        } else {
                            Text("Limits not reported").font(.system(size: 12)).foregroundStyle(.secondary)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }.frame(minHeight: 44)
                }
            }.frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}
