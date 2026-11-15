import Foundation

struct UsageGaugeRow: Identifiable {
    let id: Int
    let items: [UsageTileItem]
    let metricRows: Int
    var height: CGFloat { 154 + CGFloat(metricRows - 1) * 80 }

    static func rows(from items: [UsageTileItem], columns: Int) -> [UsageGaugeRow] {
        guard columns > 0 else { return [] }
        return stride(from: 0, to: items.count, by: columns).enumerated().map { index, start in
            let row = Array(items[start..<min(start + columns, items.count)])
            let windows = row.compactMap { $0.row?.windows?.count }.max() ?? 0
            return UsageGaugeRow(id: index, items: row, metricRows: max(1, (windows + 1) / 2))
        }
    }

    static func contentHeight(_ rows: [UsageGaugeRow]) -> CGFloat {
        rows.reduce(0) { $0 + $1.height } + CGFloat(max(0, rows.count - 1)) * 10 + 32
    }
}
