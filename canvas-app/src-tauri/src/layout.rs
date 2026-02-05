//! Time-based layout algorithm for canvas nodes

use crate::models::{CanvasNode, NodeType, Position};

/// Layout configuration
const NODE_WIDTH: f64 = 180.0;
const HORIZONTAL_GAP: f64 = 60.0;
const LANE_HEIGHT: f64 = 150.0;

/// Lane assignment for different node types
fn get_lane(node_type: &NodeType) -> usize {
    match node_type {
        NodeType::Config => 0,      // Top lane
        NodeType::Connection => 0,  // Top lane (with config)
        NodeType::Document => 1,    // Middle lane
        NodeType::Task => 2,        // Lower middle
        NodeType::Session => 3,     // Bottom lane
    }
}

/// Apply time-based layout to nodes
///
/// Nodes are arranged:
/// - X axis: sorted by timestamp (left = oldest, right = newest)
/// - Y axis: grouped by type (lanes)
pub fn apply_time_layout(nodes: &mut [CanvasNode]) {
    if nodes.is_empty() {
        return;
    }

    // Group nodes by lane index
    let mut lane_indices: [Vec<usize>; 4] = [Vec::new(), Vec::new(), Vec::new(), Vec::new()];

    for (idx, node) in nodes.iter().enumerate() {
        let lane_idx = get_lane(&node.node_type);
        if lane_idx < lane_indices.len() {
            lane_indices[lane_idx].push(idx);
        }
    }

    // Sort each lane by timestamp and assign positions
    for (lane_idx, indices) in lane_indices.iter_mut().enumerate() {
        // Sort indices by node timestamp
        indices.sort_by(|&a, &b| {
            let ts_a = nodes[a].timestamp.unwrap_or(0);
            let ts_b = nodes[b].timestamp.unwrap_or(0);
            ts_a.cmp(&ts_b)
        });

        // Calculate Y position for this lane
        let base_y = lane_idx as f64 * LANE_HEIGHT;

        // Assign X positions based on order
        for (order, &node_idx) in indices.iter().enumerate() {
            nodes[node_idx].position = Position {
                x: order as f64 * (NODE_WIDTH + HORIZONTAL_GAP),
                y: base_y,
            };
        }
    }
}

/// Prevent overlapping nodes within the same lane
pub fn resolve_overlaps(nodes: &mut [CanvasNode]) {
    // Group by Y position (approximate lane)
    let mut by_lane: std::collections::HashMap<i32, Vec<usize>> = std::collections::HashMap::new();

    for (idx, node) in nodes.iter().enumerate() {
        let lane_key = (node.position.y / LANE_HEIGHT).round() as i32;
        by_lane.entry(lane_key).or_default().push(idx);
    }

    // Sort each lane by X and resolve overlaps
    for indices in by_lane.values_mut() {
        indices.sort_by(|&a, &b| {
            nodes[a].position.x.partial_cmp(&nodes[b].position.x).unwrap()
        });

        for i in 1..indices.len() {
            let prev_idx = indices[i - 1];
            let curr_idx = indices[i];
            let prev_x = nodes[prev_idx].position.x;
            let curr_x = nodes[curr_idx].position.x;

            let min_x = prev_x + NODE_WIDTH + HORIZONTAL_GAP;
            if curr_x < min_x {
                nodes[curr_idx].position.x = min_x;
            }
        }
    }
}
