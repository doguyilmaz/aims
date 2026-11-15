import Foundation
import Testing

@testable import UsageCore

@Test func resetBoundaryHidesThePreviousPeriodWithoutInventingZero() throws {
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-15T03:00:00Z","providers":[{"id":"claude","name":"Claude","installed":true,"rows":[
              {"id":"work","name":"work","windows":[{"label":"7d","pct":97,"resetsAt":"2026-11-15T04:00:00Z"},{"label":"5h","pct":0}]}]}]}
            """#.utf8))
    let windows = try #require(snapshot.providers[0].rows[0].windows)
    let reset = try #require(windows[0].resetsAt)
    #expect(windows[0].periodPercent(at: reset.addingTimeInterval(-1)) == 97)
    #expect(windows[0].periodPercent(at: reset) == nil)
    #expect(windows[0].periodPercent(at: reset.addingTimeInterval(3600)) == nil)
    #expect(windows[0].validPercent == 97)
    #expect(windows[1].periodPercent(at: reset) == 0)
}
