import SwiftUI
import UsageCore

struct CompactProviderCard: View {
    let provider: UsageProvider
    let surface: UsageSurface
    @State private var expandedResets = false

    var body: some View {
        TimelineView(.periodic(from: .now, by: 60)) { context in
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 6) {
                    Text(ProviderStyle.title(provider)).font(.system(size: 12, weight: .semibold))
                    if let error = provider.error, !error.isEmpty {
                        Image(systemName: "exclamationmark.triangle").foregroundStyle(.orange).help(error)
                    }
                    Spacer(minLength: 4)
                    if provider.id == "codex" || provider.rows.contains(where: { $0.bankedResets != nil }) {
                        Button {
                            expandedResets.toggle()
                        } label: {
                            Label("Resets", systemImage: "tray.full")
                        }.buttonStyle(.plain).foregroundStyle(.teal)
                            .accessibilityLabel("Banked reset details for \(ProviderStyle.title(provider))")
                    }
                }.font(.system(size: 10))
                if provider.rows.isEmpty {
                    if provider.id == "claude" || provider.id == "codex" {
                        Text("Choose your aims data folder in Settings, or run aims init.")
                            .font(.system(size: 11)).foregroundStyle(.secondary)
                    }
                    Text(
                        provider.id == "claude" || provider.id == "codex"
                            ? "No accounts configured" : provider.installed ? "No reading yet" : "CLI unavailable"
                    )
                    .font(.system(size: 11)).foregroundStyle(.secondary)
                }
                ForEach(provider.rows) { row in
                    HStack(alignment: .center, spacing: 10) {
                        VStack(alignment: .leading, spacing: 3) {
                            HStack(spacing: 3) {
                                Text(ProviderStyle.accountName(row)).lineLimit(2).truncationMode(.middle)
                                if row.isStale(at: context.date) || row.error != nil {
                                    Image(systemName: "clock").font(.system(size: 9)).help("Cached reading")
                                }
                            }
                            if let balance = row.bankedResets {
                                Text("\(balance.available) banked").font(.system(size: 9)).foregroundStyle(.teal)
                            }
                        }
                        .font(.system(size: 11)).foregroundStyle(.secondary)
                        .frame(width: provider.id == "opencode" ? 108 : 77, alignment: .leading)
                        .help(
                            row.observedAt.map {
                                "\(row.name) · Reported \($0.formatted(date: .abbreviated, time: .shortened))"
                            }
                                ?? "Observation time not reported")
                        if (row.windows ?? []).isEmpty {
                            Text("Not reported").font(.system(size: 11)).foregroundStyle(.secondary)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        } else {
                            LazyVGrid(
                                columns: Array(
                                    repeating: GridItem(.flexible(), spacing: 12),
                                    count: min(2, row.windows?.count ?? 1)), spacing: 8
                            ) {
                                ForEach(Array((row.windows ?? []).enumerated()), id: \.offset) { _, window in
                                    metric(
                                        window, now: context.date,
                                        stale: row.isStale(at: context.date) || row.error != nil)
                                }
                            }.frame(maxWidth: .infinity)
                        }
                    }.frame(maxWidth: .infinity, minHeight: 25, alignment: .leading)
                    if let detail = row.detail, !detail.isEmpty {
                        Text(detail).font(.system(size: 10)).foregroundStyle(.secondary).lineLimit(3).help(detail)
                    }
                }
                if expandedResets {
                    Divider()
                    ForEach(provider.rows) { row in
                        Text(ProviderStyle.accountName(row)).font(.system(size: 11, weight: .medium))
                        ResetInventoryView(balance: row.bankedResets)
                    }
                }
                if surface == .popover { Divider().opacity(0.5) }
            }
            .padding(.horizontal, surface == .panel ? 12 : 2)
            .padding(.vertical, surface == .panel ? 10 : 4)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background {
                if surface == .panel {
                    RoundedRectangle(cornerRadius: 13, style: .continuous)
                        .fill(Color(nsColor: .controlBackgroundColor))
                        .overlay(RoundedRectangle(cornerRadius: 13).strokeBorder(.primary.opacity(0.055)))
                }
            }
        }
    }

    private func metric(_ window: UsageWindow, now: Date, stale: Bool) -> some View {
        let passed = window.needsRefresh(at: now)
        let percent = window.validPercent
        let color: Color =
            stale || passed
            ? .secondary
            : window.limited == true || (percent ?? 0) >= 95
                ? .red
                : (percent ?? 0) >= 85 ? .orange : .primary.opacity(0.6)
        return VStack(spacing: 5) {
            HStack(spacing: 4) {
                Text(window.label == "month" ? "Month" : window.label).foregroundStyle(.secondary)
                Spacer(minLength: 0)
                Text(
                    percent.map { $0 >= 1_000 ? "999+%" : String(format: "%.0f%%", $0) }
                        ?? (window.limited == true ? "Blocked" : "—")
                )
                .fontWeight(.medium).monospacedDigit().fixedSize()
                Text(resetLabel(window, now: now)).foregroundStyle(passed ? .orange : .secondary)
            }.font(.system(size: 10)).lineLimit(1)
            GeometryReader { geometry in
                Capsule().fill(.primary.opacity(0.07))
                    .overlay(alignment: .leading) {
                        Capsule().fill(color)
                            .frame(width: geometry.size.width * min(1, max(0, (percent ?? 0) / 100)))
                    }
            }.frame(height: 3).accessibilityHidden(true)
        }
        .help(
            window.resetsAt.map {
                passed ? "Reset passed · refresh needed" : "Resets \($0.formatted(date: .complete, time: .complete))"
            }
                ?? "Reset time not reported"
        )
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            "\(window.label), \(percent.map { String(format: "%.0f percent used", $0) } ?? (window.limited == true ? "blocked, percentage unavailable" : "usage unavailable")), \(passed ? "reset passed, refresh needed" : resetLabel(window, now: now))"
        )
    }

    private func resetLabel(_ window: UsageWindow, now: Date) -> String {
        guard let date = window.resetsAt else { return "—" }
        if date <= now { return "Refresh" }
        return ResetPresentation.countdown(to: date, now: now)
            .replacingOccurrences(of: "in ", with: "")
            .replacingOccurrences(of: " 0m", with: "")
            .replacingOccurrences(of: " 0h", with: "")
    }
}
