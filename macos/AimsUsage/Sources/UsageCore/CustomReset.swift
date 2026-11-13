import Foundation

public struct CustomReset: Decodable, Identifiable, Sendable {
    public let id: String
    public let provider: String
    public let row: String
    public let label: String
    public let at: Date
}
