import Foundation
import Testing
import UsageCore

@testable import AimsUsage

@Test @MainActor func extraWindowsGrowOnlyTheirOwnGridRowWithoutDroppingAccounts() throws {
    let rows: [[String: Any]] = (0..<10).map { index in
        [
            "id": "account-\(index)", "name": "Account \(index)",
            "windows": (0..<(index == 0 ? 8 : 1)).map { ["label": "window-\($0)", "pct": 25] },
        ]
    }
    let data = try JSONSerialization.data(withJSONObject: [
        "schemaVersion": 1,
        "generatedAt": "2026-11-13T00:00:00Z",
        "providers": [["id": "codex", "name": "Codex", "installed": true, "rows": rows]],
    ])
    let snapshot = try UsageSnapshot.decode(data)
    let items = UsageTileItem.items(from: snapshot)
    let grid = UsageGaugeRow.rows(from: items, columns: 2)
    #expect(grid.count == 5)
    #expect(grid[0].metricRows == 4)
    #expect(grid.dropFirst().allSatisfy { $0.metricRows == 1 })
    #expect(grid.flatMap(\.items).map(\.id) == items.map(\.id))
    #expect(UsageGaugeRow.contentHeight(grid) < 1_100)
    #expect(UsageGaugeRow.rows(from: items, columns: 3).flatMap(\.items).count == 10)
}
