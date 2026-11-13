import Foundation

public struct UsageProvider: Decodable, Identifiable, Sendable {
    public let id: String
    public let name: String
    public let installed: Bool
    public let rows: [UsageRow]
    public let error: String?
    public let attemptedAt: Date?
}
