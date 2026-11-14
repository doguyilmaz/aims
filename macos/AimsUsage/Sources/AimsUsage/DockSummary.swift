import Foundation
import UsageCore

struct DockSummary {
    private enum Reading {
        case current(Double)
        case blocked(Double?)
        case cached(Double, Date?)
        case unavailable(String)
    }

    private let reading: Reading
    private let focus: DockWindowFocus
    private let valueMode: DockValueMode

    init(
        provider: UsageProvider, now: Date, globalError: String? = nil,
        focus: DockWindowFocus = .automatic, valueMode: DockValueMode = .used
    ) {
        self.focus = focus
        self.valueMode = valueMode
        if let error = globalError ?? provider.error {
            reading = .unavailable(error)
            return
        }
        let readings = provider.rows.filter { row in
            row.error == nil && (row.observedAt.map { $0 <= now.addingTimeInterval(60) } ?? true)
        }.flatMap { row in
            (row.windows ?? []).filter { focus.includes($0) && !$0.needsRefresh(at: now) }
                .map { (row: row, window: $0) }
        }
        let current = readings.filter { !$0.row.isStale(at: now) }.map(\.window)
        let currentPercent = current.compactMap(\.validPercent).max()
        if current.contains(where: { $0.limited == true }) {
            reading = .blocked(currentPercent)
        } else if let currentPercent {
            reading = .current(currentPercent)
        } else if let cached = readings.compactMap({ item -> (percent: Double, observedAt: Date?)? in
            guard let percent = item.window.validPercent else { return nil }
            return (percent, item.row.observedAt)
        }).max(by: { $0.percent < $1.percent }) {
            reading = .cached(cached.percent, cached.observedAt)
        } else {
            reading = .unavailable(
                "\(focus == .automatic ? "Usage percentage" : focus.title + " usage") not reported for an active window. Hover for all readings."
            )
        }
    }

    var percent: Double? {
        switch reading {
        case .current(let percent), .cached(let percent, _): percent
        case .blocked(let percent): percent
        case .unavailable: nil
        }
    }

    var currentPercent: Double? { isCached ? nil : percent }
    var displayPercent: Double? { percent.map(valueMode.percent) }
    var blocked: Bool { if case .blocked = reading { true } else { false } }
    var isCached: Bool { if case .cached = reading { true } else { false } }

    var explanation: String {
        switch reading {
        case .cached(_, let date):
            "\(selectionDescription) · Last known usage · \(date.map { "reported \($0.formatted(date: .abbreviated, time: .shortened))" } ?? "observation time unavailable")"
        case .unavailable(let reason): reason
        case .current, .blocked: "\(selectionDescription) · Current reading"
        }
    }

    var value: String {
        if blocked { return "Blocked" }
        return displayPercent.map { $0 >= 1_000 ? "999+%" : String(format: "%.0f%%", $0) } ?? "—"
    }

    private var selectionDescription: String {
        "\(valueMode.selectionDescription) across accounts · \(focus == .automatic ? "all windows" : focus.title)"
    }
}
