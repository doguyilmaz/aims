import SwiftUI

struct EdgeDockShape: Shape {
    func path(in rect: CGRect) -> Path {
        let edge = rect.maxX
        let curve: CGFloat = 24
        let radius: CGFloat = 22
        var path = Path()
        path.move(to: CGPoint(x: edge, y: rect.minY))
        path.addCurve(
            to: CGPoint(x: edge - curve, y: rect.minY + curve),
            control1: CGPoint(x: edge, y: rect.minY + curve),
            control2: CGPoint(x: edge - 8, y: rect.minY + curve))
        path.addLine(to: CGPoint(x: rect.minX + radius, y: rect.minY + curve))
        path.addQuadCurve(
            to: CGPoint(x: rect.minX, y: rect.minY + curve + radius),
            control: CGPoint(x: rect.minX, y: rect.minY + curve))
        path.addLine(to: CGPoint(x: rect.minX, y: rect.maxY - curve - radius))
        path.addQuadCurve(
            to: CGPoint(x: rect.minX + radius, y: rect.maxY - curve),
            control: CGPoint(x: rect.minX, y: rect.maxY - curve))
        path.addLine(to: CGPoint(x: edge - curve, y: rect.maxY - curve))
        path.addCurve(
            to: CGPoint(x: edge, y: rect.maxY),
            control1: CGPoint(x: edge - 8, y: rect.maxY - curve),
            control2: CGPoint(x: edge, y: rect.maxY - curve))
        path.closeSubpath()
        return path
    }
}
