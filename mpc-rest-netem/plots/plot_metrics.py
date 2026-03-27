import matplotlib.pyplot as plt
import numpy as np

# Các kịch bản thử nghiệm mạng
scenarios = ['Baseline (250ms)', 'High Latency (4s delay)', 'Flaky (30% Drop)']

# Dữ liệu thực tế: Baseline 2.58s | Delay 55.31s | Flaky chạm trần 600s (Timeout)
t_sign_seconds = [2.58, 55.31, 600]

plt.figure(figsize=(8, 5))
# Sử dụng màu Xanh (Bình thường) - Cam (Chậm) - Xám (Thất bại/Timeout)
bars = plt.bar(scenarios, t_sign_seconds, color=['#4CAF50', '#FF9800', '#9E9E9E'], width=0.5)

for i, bar in enumerate(bars):
    yval = bar.get_height()
    if i == 2: # Xử lý riêng cho cột Timeout
        plt.text(bar.get_x() + bar.get_width()/2, yval + 10, 'Timeout\n(>600s)', ha='center', va='bottom', fontweight='bold', color='red')
    else:
        plt.text(bar.get_x() + bar.get_width()/2, yval + 10, f'{yval:.2f}s', ha='center', va='bottom', fontweight='bold')

plt.title('Impact of Network Anomalies on MPC Signing Time ($T_{sign}$)', fontsize=14, pad=20)
plt.ylabel('Execution Time (Seconds)', fontsize=12)
plt.xlabel('Network Scenario', fontsize=12)
plt.ylim(0, 680) # Nới rộng trục Y để hiển thị chữ Timeout không bị cắt
plt.grid(axis='y', linestyle='--', alpha=0.7)

plt.tight_layout()
plt.savefig('mpc_network_evaluation_final.png', dpi=300)
print("Đã xuất biểu đồ ra file mpc_network_evaluation_final.png")