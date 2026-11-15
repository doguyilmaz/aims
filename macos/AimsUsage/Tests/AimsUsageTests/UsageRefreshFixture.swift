import Foundation

@MainActor
final class UsageRefreshFixture {
    var percent = 100
    var failProvider = true
    var failCache = false
    var calls: [Bool] = []
    var ready: CheckedContinuation<Void, Never>?
    var response: CheckedContinuation<Data, any Error>?

    func reading() -> Data {
        Data(
            """
            {"schemaVersion":1,"generatedAt":"2026-11-15T20:00:00Z","providers":[{"id":"codex","name":"Codex","installed":true,"rows":[{"id":"work","name":"work","observedAt":"2026-11-15T20:00:00Z","windows":[{"label":"7d","pct":\(percent)}]}]}]}
            """.utf8)
    }
}
