<?php
namespace mail\mailflow;

use app\admin\lib\Plugin;

class MailflowPlugin extends Plugin
{
    public $info = array(
        'name'        => 'Mailflow',
        'title'       => 'Mailflow邮件发送',
        'description' => '通过Mailflow API发送邮件',
        'status'      => 1,
        'author'      => 'xkatld',
        'version'     => '1.0.0',
        'help_url'    => 'https://github.com/xkatld/MailFlow',
    );

    const LOG_FILE = __DIR__ . '/mailflow_debug.log';

    private $isDebug = true;

    public function install()
    {
        return true;
    }

    public function uninstall()
    {
        return true;
    }

    public function send($params)
    {
        $this->writeLog("==================== 开始发送邮件 ====================");
        $this->writeLog("收件人: " . $params['email']);
        $this->writeLog("主题: " . ($params['subject'] ?? '无'));

        if (empty($params['email'])) {
            $this->writeLog("发送失败: 收件人地址为空");
            return ['status' => 'error', 'msg' => '收件人地址不能为空'];
        }

        if (empty($params['subject'])) {
            $this->writeLog("发送失败: 邮件主题为空");
            return ['status' => 'error', 'msg' => '邮件主题不能为空'];
        }

        if (empty($params['content'])) {
            $this->writeLog("发送失败: 邮件内容为空");
            return ['status' => 'error', 'msg' => '邮件内容不能为空'];
        }

        $config = $params['config'];
        
        if (empty($config['api_url'])) {
            $this->writeLog("发送失败: API地址未配置");
            return ['status' => 'error', 'msg' => 'Mailflow API地址未配置'];
        }

        if (empty($config['api_key'])) {
            $this->writeLog("发送失败: API Key未配置");
            return ['status' => 'error', 'msg' => 'Mailflow API Key未配置'];
        }

        $requestData = [
            'to'      => [$params['email']],
            'subject' => $params['subject'],
            'html'    => $params['content'],
        ];

        if (!empty($params['text'])) {
            $requestData['text'] = $params['text'];
        }

        $this->writeLog("请求数据: " . json_encode($requestData, JSON_UNESCAPED_UNICODE));

        $result = $this->callAPI($config, $requestData);

        if ($result === null) {
            $this->writeLog("发送失败: 无法连接到Mailflow服务器");
            return ['status' => 'error', 'msg' => '无法连接到Mailflow服务器'];
        }

        if (isset($result['error'])) {
            $this->writeLog("发送失败: " . $result['error']);
            return ['status' => 'error', 'msg' => $result['error']];
        }

        if (isset($result['message']) && strpos($result['message'], '已加入发送队列') !== false) {
            $this->writeLog("发送成功: 邮件已加入发送队列");
            return ['status' => 'success'];
        }

        if (isset($result['code']) && $result['code'] !== 200) {
            $errorMsg = $result['msg'] ?? $result['error'] ?? '发送失败';
            $this->writeLog("发送失败: " . $errorMsg);
            return ['status' => 'error', 'msg' => $errorMsg];
        }

        $this->writeLog("发送成功");
        return ['status' => 'success'];
    }

    private function callAPI($config, $data)
    {
        $apiUrl = rtrim($config['api_url'], '/');
        $url = $apiUrl . '/api/v1/send';
        $apiKey = $config['api_key'];
        $timeout = intval($config['timeout'] ?? 30);

        $this->writeLog("API请求URL: " . $url);
        $this->writeLog("超时时间: " . $timeout . "秒");

        $ch = curl_init();
        
        $headers = [
            'X-API-Key: ' . $apiKey,
            'Content-Type: application/json',
            'Accept: application/json',
        ];

        curl_setopt($ch, CURLOPT_URL, $url);
        curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
        curl_setopt($ch, CURLOPT_POST, true);
        curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($data));
        curl_setopt($ch, CURLOPT_HTTPHEADER, $headers);
        curl_setopt($ch, CURLOPT_TIMEOUT, $timeout);
        curl_setopt($ch, CURLOPT_CONNECTTIMEOUT, 10);
        curl_setopt($ch, CURLOPT_SSL_VERIFYPEER, false);
        curl_setopt($ch, CURLOPT_SSL_VERIFYHOST, false);

        $response = curl_exec($ch);
        $httpCode = curl_getinfo($ch, CURLINFO_HTTP_CODE);
        $errno = curl_errno($ch);
        $error = curl_error($ch);

        curl_close($ch);

        $this->writeLog("HTTP状态码: " . $httpCode);
        $this->writeLog("响应内容: " . $response);

        if ($errno) {
            $this->writeLog("CURL错误[{$errno}]: " . $error);
            return null;
        }

        $result = json_decode($response, true);
        
        if (json_last_error() !== JSON_ERROR_NONE) {
            $this->writeLog("JSON解析错误: " . json_last_error_msg());
            $this->writeLog("原始响应: " . $response);
            return ['error' => 'API响应格式错误: ' . json_last_error_msg()];
        }

        return $result;
    }

    private function writeLog($message)
    {
        if (!$this->isDebug) {
            return;
        }

        $time = date('Y-m-d H:i:s');
        $logMessage = "[{$time}] {$message}" . PHP_EOL;
        
        try {
            file_put_contents(self::LOG_FILE, $logMessage, FILE_APPEND);
        } catch (\Exception $e) {
        }
    }
}

